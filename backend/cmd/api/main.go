package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Server struct {
	db            *pgxpool.Pool
	secure        bool
	allowedOrigin string
	invitationKey string
	resendAPIKey  string
	resendFrom    string
}

func (s *Server) sendEmail(to, subject, html string) error {
	if s.resendAPIKey == "" || s.resendFrom == "" {
		return errors.New("email is not configured")
	}
	payload, _ := json.Marshal(map[string]any{"from": s.resendFrom, "to": []string{to}, "subject": subject, "html": html})
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.resendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend returned %s", resp.Status)
	}
	return nil
}

type Operator struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}
type SchoolInstance struct {
	ID         string
	SchoolID   string
	InstanceID string
}
type key int

const actorKey key = 0

func id() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func digest(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }
func invitationCipherKey(key string) []byte {
	h := sha256.Sum256([]byte("educraft360 invitation encryption:" + key))
	return h[:]
}
func encryptInvitation(raw, key string) (string, error) {
	block, err := aes.NewCipher(invitationCipherKey(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(raw), nil)), nil
}
func decryptInvitation(encoded, key string) (string, error) {
	block, err := aes.NewCipher(invitationCipherKey(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(data) < gcm.NonceSize() {
		return "", errors.New("invalid invitation ciphertext")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
func signInvitation(payload, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func invitationToken(invitationID, schoolID string, expiresAt time.Time, key string) string {
	payload := strings.Join([]string{invitationID, schoolID, strconv.FormatInt(expiresAt.Unix(), 10), token()[:16]}, ".")
	return payload + "." + signInvitation(payload, key)
}
func validInvitationToken(raw, key string) (string, string, time.Time, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) != 5 || key == "" {
		return "", "", time.Time{}, false
	}
	payload := strings.Join(parts[:4], ".")
	if !hmac.Equal([]byte(parts[4]), []byte(signInvitation(payload, key))) {
		return "", "", time.Time{}, false
	}
	seconds, e := strconv.ParseInt(parts[2], 10, 64)
	if e != nil {
		return "", "", time.Time{}, false
	}
	expiresAt := time.Unix(seconds, 0)
	return parts[0], parts[1], expiresAt, true
}
func schoolInstallURL(domain, rawToken, instanceID string) (string, bool) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "", false
	}
	if !strings.Contains(domain, "://") {
		domain = "https://" + domain
	}
	parsed, err := url.Parse(domain)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", false
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/install"
	parsed.Fragment = "instance_id=" + url.QueryEscape(instanceID) + "&invitation=" + url.QueryEscape(rawToken)
	return parsed.String(), true
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, s string) {
	reply(w, status, map[string]string{"error": s})
}
func body(r *http.Request, d any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(d)
}
func textOK(s string, max int) bool { return len(strings.TrimSpace(s)) > 0 && len(s) <= max }
func generatedSchoolCode(name, uid string) string {
	var slug strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			slug.WriteRune(r)
		} else if slug.Len() > 0 && !strings.HasSuffix(slug.String(), "-") {
			slug.WriteByte('-')
		}
	}
	code := strings.Trim(slug.String(), "-")
	if code == "" {
		code = "SCHOOL"
	}
	if len(code) > 28 {
		code = strings.TrimRight(code[:28], "-")
	}
	return code + "-" + strings.ToUpper(strings.ReplaceAll(uid[:6], "-", ""))
}
func (s *Server) audit(ctx context.Context, o Operator, action, typ, entity string) {
	_, e := s.db.Exec(ctx, "INSERT INTO audit_events(id,actor_id,action,entity_type,entity_id) VALUES($1,$2,$3,$4,$5)", id(), o.ID, action, typ, entity)
	if e != nil {
		log.Printf("audit: %v", e)
	}
}
func (s *Server) guard(next http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("educraft_session")
		if e != nil {
			fail(w, 401, "login required")
			return
		}
		o := Operator{}
		e = s.db.QueryRow(r.Context(), `SELECT o.id,o.name,o.email,o.role FROM sessions ss JOIN operators o ON ss.operator_id=o.id WHERE ss.token_hash=$1 AND ss.expires_at>NOW() AND o.active=TRUE`, digest(c.Value)).Scan(&o.ID, &o.Name, &o.Email, &o.Role)
		if e != nil {
			fail(w, 401, "session expired")
			return
		}
		if len(roles) > 0 {
			ok := false
			for _, role := range roles {
				if o.Role == role {
					ok = true
				}
			}
			if !ok {
				fail(w, 403, "insufficient permission")
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), actorKey, o)))
	}
}
func (s *Server) schoolGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			fail(w, 401, "school instance authentication required")
			return
		}
		var instance SchoolInstance
		e := s.db.QueryRow(r.Context(), `SELECT id::text,school_id::text,instance_id FROM school_instance_credentials WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>NOW()`, digest(parts[1])).Scan(&instance.ID, &instance.SchoolID, &instance.InstanceID)
		if e != nil {
			fail(w, 401, "invalid or expired school instance credential")
			return
		}
		_, _ = s.db.Exec(r.Context(), "UPDATE school_instance_credentials SET last_used_at=NOW() WHERE id=$1", instance.ID)
		r = r.WithContext(context.WithValue(r.Context(), schoolInstanceKey{}, instance))
		next(w, r)
	}
}

type schoolInstanceKey struct{}

func schoolInstanceFrom(r *http.Request) SchoolInstance {
	instance, _ := r.Context().Value(schoolInstanceKey{}).(SchoolInstance)
	return instance
}
func actor(r *http.Request) Operator { return r.Context().Value(actorKey).(Operator) }
func (s *Server) origin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if origin != "" && !s.originAllowed(origin) {
				fail(w, 403, "untrusted origin")
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				fail(w, 415, "JSON required")
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) originAllowed(origin string) bool {
	if origin == s.allowedOrigin {
		return true
	}
	// Local development commonly switches between localhost, 127.0.0.1,
	// and an automatically selected Next.js port. Keep production strict.
	if s.secure {
		return false
	}
	configured, configuredErr := url.Parse(s.allowedOrigin)
	requested, requestedErr := url.Parse(origin)
	if configuredErr != nil || requestedErr != nil || configured.Scheme != "http" || requested.Scheme != "http" {
		return false
	}
	return isLoopbackHost(configured.Hostname()) && isLoopbackHost(requested.Hostname())
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "[::1]" || host == "::1"
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if body(r, &x) != nil {
		fail(w, 400, "invalid request")
		return
	}
	var o Operator
	var hash string
	err := s.db.QueryRow(r.Context(), "SELECT id,name,email,role,password_hash FROM operators WHERE email=$1 AND active=true", strings.ToLower(strings.TrimSpace(x.Email))).Scan(&o.ID, &o.Name, &o.Email, &o.Role, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(x.Password)) != nil {
		fail(w, 401, "invalid credentials")
		return
	}
	t := token()
	_, err = s.db.Exec(r.Context(), "INSERT INTO sessions(token_hash,operator_id,expires_at) VALUES($1,$2,$3)", digest(t), o.ID, time.Now().Add(12*time.Hour))
	if err != nil {
		fail(w, 500, "session unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "educraft_session", Value: t, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, Path: "/", MaxAge: 43200})
	reply(w, 200, o)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("educraft_session"); e == nil {
		_, _ = s.db.Exec(r.Context(), "DELETE FROM sessions WHERE token_hash=$1", digest(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "educraft_session", Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) { reply(w, 200, actor(r)) }
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var x struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if body(r, &x) != nil || len(x.NewPassword) < 12 {
		fail(w, 400, "new password must be at least 12 characters")
		return
	}
	o := actor(r)
	var hash string
	if e := s.db.QueryRow(r.Context(), "SELECT password_hash FROM operators WHERE id=$1 AND active", o.ID).Scan(&hash); e != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(x.CurrentPassword)) != nil {
		fail(w, 400, "current password is incorrect")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(x.NewPassword)) == nil {
		fail(w, 400, "new password must be different")
		return
	}
	newHash, e := bcrypt.GenerateFromPassword([]byte(x.NewPassword), bcrypt.DefaultCost)
	if e != nil {
		fail(w, 500, "password could not be updated")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "password could not be updated")
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "UPDATE operators SET password_hash=$1 WHERE id=$2", string(newHash), o.ID); e != nil {
		fail(w, 500, "password could not be updated")
		return
	}
	if _, e = tx.Exec(r.Context(), "DELETE FROM sessions WHERE operator_id=$1", o.ID); e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "password could not be updated")
		return
	}
	s.audit(r.Context(), o, "operator.password_changed", "operator", o.ID)
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	var schools, active, tickets, open, invoiceCount int
	var outstanding int64
	e := s.db.QueryRow(r.Context(), `
		SELECT
			(SELECT COUNT(*) FROM schools),
			(SELECT COUNT(*) FROM schools WHERE status='active'),
			(SELECT COUNT(*) FROM tickets),
			(SELECT COUNT(*) FROM tickets WHERE status NOT IN ('resolved','closed')),
			(SELECT COUNT(*) FROM invoices),
			(SELECT COALESCE(SUM(i.amount_minor - COALESCE(
				(SELECT SUM(p.amount_minor) FROM payments p WHERE p.invoice_id=i.id), 0
			)), 0)::bigint FROM invoices i WHERE i.status NOT IN ('draft','void') AND i.currency='NGN')
	`).Scan(&schools, &active, &tickets, &open, &invoiceCount, &outstanding)
	if e != nil {
		fail(w, 500, "dashboard unavailable")
		return
	}
	reply(w, 200, map[string]any{"schools": schools, "active_schools": active, "tickets": tickets, "open_tickets": open, "invoices": invoiceCount, "outstanding_minor": outstanding})
}
func (s *Server) list(w http.ResponseWriter, r *http.Request, kind string) {
	queries := map[string]string{
		"schools":          `SELECT id::text AS id,name,code,contact_name,contact_email,contact_phone,domain,hosting,status,notes,created_at FROM schools ORDER BY created_at DESC, id`,
		"plans":            `SELECT id::text AS id,name,description,amount_minor,currency,interval,active,created_at FROM plans ORDER BY created_at DESC LIMIT 200`,
		"subscriptions":    `SELECT s.id::text AS id,s.school_id::text AS school_id,sc.name AS school_name,sc.contact_email,COALESCE(STRING_AGG(DISTINCT p.name, ', ' ORDER BY p.name),'') AS plan_name,COALESCE(ARRAY_AGG(DISTINCT sp.plan_id::text) FILTER (WHERE sp.plan_id IS NOT NULL),'{}') AS plan_ids,ta.id::text AS activation_id,s.status,s.starts_on,s.ends_on,s.created_at FROM subscriptions s JOIN schools sc ON s.school_id=sc.id LEFT JOIN subscription_plans sp ON sp.subscription_id=s.id LEFT JOIN plans p ON p.id=sp.plan_id LEFT JOIN LATERAL (SELECT id FROM term_activations WHERE subscription_id=s.id ORDER BY created_at DESC LIMIT 1) ta ON true GROUP BY s.id,s.school_id,sc.name,sc.contact_email,ta.id ORDER BY s.created_at DESC LIMIT 200`,
		"invoices":         `SELECT i.id::text AS id,i.invoice_no,sc.name AS school_name,i.description,i.unit_count,i.unit_amount_minor,CASE WHEN i.unit_count > 0 AND i.unit_amount_minor > 0 THEN i.unit_count*i.unit_amount_minor ELSE i.amount_minor END AS amount_minor,i.currency,i.status,i.due_on,COALESCE((SELECT SUM(amount_minor) FROM payments WHERE invoice_id=i.id),0)::bigint AS paid_minor FROM invoices i JOIN schools sc ON i.school_id=sc.id ORDER BY i.created_at DESC LIMIT 200`,
		"tickets":          `SELECT t.id::text AS id,t.ticket_no,sc.name AS school_name,sc.code AS school_code,t.subject,t.category,t.priority,t.status,COALESCE(o.name,'') AS assignee_name,t.created_at FROM tickets t JOIN schools sc ON t.school_id=sc.id LEFT JOIN operators o ON t.assigned_to=o.id ORDER BY t.created_at DESC LIMIT 200`,
		"ticket_assignees": `SELECT id::text AS id,name FROM operators WHERE active AND role IN ('owner','admin','support') ORDER BY name,id`,
		"operators":        `SELECT id::text AS id,name,email,role,active,created_at FROM operators ORDER BY created_at DESC LIMIT 200`,
	}
	rows, e := s.db.Query(r.Context(), queries[kind])
	if e != nil {
		fail(w, 500, "query failed")
		return
	}
	defer rows.Close()
	cols := rows.FieldDescriptions()
	result := make([]map[string]any, 0)
	for rows.Next() {
		v, err := rows.Values()
		if err != nil {
			break
		}
		obj := map[string]any{}
		for i, c := range cols {
			obj[string(c.Name)] = v[i]
		}
		result = append(result, obj)
	}
	if rows.Err() != nil {
		fail(w, 500, "query interrupted")
		return
	}
	reply(w, 200, result)
}
func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		fail(w, 400, "school_id is required")
		return
	}
	rows, e := s.db.Query(r.Context(), `SELECT i.id::text, i.school_id::text, sc.name, sc.code, sc.contact_email, i.status, i.expires_at, i.instance_id, i.instance_url, i.consumed_at, i.created_at,
        CASE WHEN i.status='consumed' THEN 'consumed' WHEN i.status='revoked' THEN 'revoked' WHEN i.expires_at <= NOW() THEN 'expired' ELSE 'active' END AS availability
        FROM installation_invitations i JOIN schools sc ON sc.id=i.school_id WHERE i.school_id=$1 ORDER BY i.created_at DESC LIMIT 500`, schoolID)
	if e != nil {
		fail(w, 500, "invitations unavailable")
		return
	}
	defer rows.Close()
	cols := rows.FieldDescriptions()
	result := make([]map[string]any, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			fail(w, 500, "invitations unavailable")
			return
		}
		item := map[string]any{}
		for i, col := range cols {
			item[string(col.Name)] = values[i]
		}
		result = append(result, item)
	}
	if rows.Err() != nil {
		fail(w, 500, "invitations unavailable")
		return
	}
	reply(w, 200, result)
}
func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request) {
	if s.invitationKey == "" {
		fail(w, 503, "invitation signing is not configured")
		return
	}
	var x struct {
		SchoolID       string `json:"school_id"`
		ExpiresInHours int    `json:"expires_in_hours"`
	}
	if body(r, &x) != nil || x.ExpiresInHours < 1 || x.ExpiresInHours > 720 {
		fail(w, 400, "school_id and expiry between 1 and 720 hours required")
		return
	}
	var schoolName, schoolCode, schoolDomain string
	if e := s.db.QueryRow(r.Context(), "SELECT name,code,domain FROM schools WHERE id=$1", x.SchoolID).Scan(&schoolName, &schoolCode, &schoolDomain); e != nil {
		fail(w, 404, "school not found")
		return
	}
	if strings.TrimSpace(schoolDomain) == "" {
		fail(w, 400, "school domain is required before creating an invitation")
		return
	}
	invitationID := id()
	instanceID := "inst-" + token()[:24]
	expiresAt := time.Now().Add(time.Duration(x.ExpiresInHours) * time.Hour)
	rawToken := invitationToken(invitationID, x.SchoolID, expiresAt, s.invitationKey)
	ciphertext, e := encryptInvitation(rawToken, s.invitationKey)
	if e != nil {
		fail(w, 500, "invitation could not be created")
		return
	}
	_, e = s.db.Exec(r.Context(), `INSERT INTO installation_invitations(id,school_id,instance_id,token_hash,token_ciphertext,expires_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`, invitationID, x.SchoolID, instanceID, digest(rawToken), ciphertext, expiresAt, actor(r).ID)
	if e != nil {
		fail(w, 500, "invitation could not be created")
		return
	}
	s.audit(r.Context(), actor(r), "installation_invitation.created", "school", x.SchoolID)
	installURL, validDomain := schoolInstallURL(schoolDomain, rawToken, instanceID)
	if !validDomain {
		fail(w, 400, "school domain must be a valid hostname or URL")
		return
	}
	reply(w, 201, map[string]any{"id": invitationID, "school_id": x.SchoolID, "school_name": schoolName, "school_code": schoolCode, "instance_id": instanceID, "expires_at": expiresAt, "token": rawToken, "install_url": installURL})
}
func (s *Server) invitationLink(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var ciphertext, status, domain, instanceID string
	if e := s.db.QueryRow(r.Context(), `SELECT i.token_ciphertext,i.status,sc.domain,i.instance_id FROM installation_invitations i JOIN schools sc ON sc.id=i.school_id WHERE i.id=$1`, id).Scan(&ciphertext, &status, &domain, &instanceID); e != nil {
		fail(w, 404, "invitation not found")
		return
	}
	if status == "revoked" {
		fail(w, 409, "invitation has been revoked")
		return
	}
	token, e := decryptInvitation(ciphertext, s.invitationKey)
	if e != nil {
		fail(w, 500, "invitation link unavailable")
		return
	}
	installURL, valid := schoolInstallURL(domain, token, instanceID)
	if !valid {
		fail(w, 500, "school domain is invalid")
		return
	}
	reply(w, 200, map[string]string{"install_url": installURL})
}
func (s *Server) emailInvitation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var ciphertext, status, domain, email, schoolName, instanceID string
	if e := s.db.QueryRow(r.Context(), `SELECT i.token_ciphertext,i.status,sc.domain,sc.contact_email,sc.name,i.instance_id FROM installation_invitations i JOIN schools sc ON sc.id=i.school_id WHERE i.id=$1`, id).Scan(&ciphertext, &status, &domain, &email, &schoolName, &instanceID); e != nil {
		fail(w, 404, "invitation not found")
		return
	}
	if status == "revoked" {
		fail(w, 409, "invitation has been revoked")
		return
	}
	token, e := decryptInvitation(ciphertext, s.invitationKey)
	if e != nil {
		fail(w, 409, "invitation link unavailable")
		return
	}
	installURL, valid := schoolInstallURL(domain, token, instanceID)
	if !valid || strings.TrimSpace(email) == "" {
		fail(w, 400, "school email or domain is unavailable")
		return
	}
	html := fmt.Sprintf("<p>Hello %s,</p><p>Use the link below to install your EduCraft360 school instance.</p><p><a href=\"%s\">Open installation</a></p><p>This link is for your school and may only be used once.</p>", htmlEscape(schoolName), htmlEscape(installURL))
	if e = s.sendEmail(email, "EduCraft360 school installation", html); e != nil {
		fail(w, 503, e.Error())
		return
	}
	reply(w, 200, map[string]any{"sent": true, "to": email})
}
func htmlEscape(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(value)
}
func (s *Server) verifyInvitation(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Token       string `json:"token"`
		InstanceID  string `json:"instance_id"`
		InstanceURL string `json:"instance_url"`
	}
	if body(r, &x) != nil || !textOK(x.Token, 200) || !textOK(x.InstanceID, 160) {
		fail(w, 400, "token and instance_id required")
		return
	}
	invitationID, schoolID, expiresAt, ok := validInvitationToken(x.Token, s.invitationKey)
	if !ok || expiresAt.Before(time.Now()) {
		fail(w, 401, "invalid or expired invitation")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "invitation unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	var storedInstance string
	var storedSchool string
	e = tx.QueryRow(r.Context(), `SELECT school_id::text,status,instance_id FROM installation_invitations WHERE id=$1 AND token_hash=$2 FOR UPDATE`, invitationID, digest(x.Token)).Scan(&storedSchool, &status, &storedInstance)
	if e != nil || storedSchool != schoolID || status != "approved" {
		fail(w, 409, "invitation is unavailable")
		return
	}
	if storedInstance != "" && storedInstance != x.InstanceID {
		fail(w, 409, "invitation is bound to another instance")
		return
	}
	if _, e = tx.Exec(r.Context(), "UPDATE installation_invitations SET instance_id=COALESCE(NULLIF(instance_id,''),$1),instance_url=COALESCE(NULLIF(instance_url,''),$2) WHERE id=$3", x.InstanceID, strings.TrimSpace(x.InstanceURL), invitationID); e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "invitation could not be verified")
		return
	}
	var schoolName, schoolCode string
	if e = s.db.QueryRow(r.Context(), "SELECT name,code FROM schools WHERE id=$1", schoolID).Scan(&schoolName, &schoolCode); e != nil {
		fail(w, 404, "school not found")
		return
	}
	reply(w, 200, map[string]any{"valid": true, "invitation_id": invitationID, "school_id": schoolID, "school_name": schoolName, "school_code": schoolCode, "instance_id": x.InstanceID, "expires_at": expiresAt})
}
func (s *Server) consumeInvitation(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Token      string `json:"token"`
		InstanceID string `json:"instance_id"`
	}
	if body(r, &x) != nil || !textOK(x.Token, 200) || !textOK(x.InstanceID, 160) {
		fail(w, 400, "token and instance_id required")
		return
	}
	invitationID, schoolID, expiresAt, ok := validInvitationToken(x.Token, s.invitationKey)
	if !ok || expiresAt.Before(time.Now()) {
		fail(w, 401, "invalid or expired invitation")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "invitation unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	var storedSchool, status, storedInstance string
	e = tx.QueryRow(r.Context(), `SELECT school_id::text,status,instance_id FROM installation_invitations WHERE id=$1 AND token_hash=$2 FOR UPDATE`, invitationID, digest(x.Token)).Scan(&storedSchool, &status, &storedInstance)
	if e != nil || storedSchool != schoolID || status != "approved" || (storedInstance != "" && storedInstance != x.InstanceID) {
		fail(w, 409, "invitation is unavailable")
		return
	}
	if _, e = tx.Exec(r.Context(), "UPDATE installation_invitations SET status='consumed',consumed_at=NOW(),instance_id=$1 WHERE id=$2", x.InstanceID, invitationID); e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "invitation could not be consumed")
		return
	}
	apiToken := token()
	credentialID := id()
	credentialExpiry := time.Now().AddDate(1, 0, 0)
	if _, e = s.db.Exec(r.Context(), "INSERT INTO school_instance_credentials(id,school_id,instance_id,token_hash,expires_at) VALUES($1,$2,$3,$4,$5)", credentialID, schoolID, x.InstanceID, digest(apiToken), credentialExpiry); e != nil {
		fail(w, 500, "school credential could not be issued")
		return
	}
	reply(w, 200, map[string]any{"consumed": true, "invitation_id": invitationID, "school_id": schoolID, "instance_id": x.InstanceID, "credential_id": credentialID, "api_token": apiToken, "api_token_expires_at": credentialExpiry})
}
func (s *Server) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	invitationID := strings.TrimSpace(r.PathValue("id"))
	if invitationID == "" {
		fail(w, 400, "invitation id is required")
		return
	}
	tag, e := s.db.Exec(r.Context(), "UPDATE installation_invitations SET status='revoked' WHERE id=$1 AND status='approved' AND expires_at > NOW()", invitationID)
	if e != nil {
		fail(w, 500, "invitation could not be revoked")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "only approved, unused, unexpired invitations can be revoked")
		return
	}
	s.audit(r.Context(), actor(r), "installation_invitation.revoked", "installation_invitation", invitationID)
	reply(w, 200, map[string]any{"revoked": true, "invitation_id": invitationID})
}
func (s *Server) createSchool(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Name         string `json:"name"`
		Code         string `json:"code"`
		ContactName  string `json:"contact_name"`
		ContactEmail string `json:"contact_email"`
		ContactPhone string `json:"contact_phone"`
		Domain       string `json:"domain"`
		Hosting      string `json:"hosting"`
		Notes        string `json:"notes"`
	}
	if body(r, &x) != nil || !textOK(x.Name, 180) || (x.Code != "" && len(strings.TrimSpace(x.Code)) > 40) {
		fail(w, 400, "valid school name required")
		return
	}
	if x.Hosting == "" {
		x.Hosting = "self_hosted"
	}
	if x.Hosting != "self_hosted" && x.Hosting != "techknights" {
		fail(w, 400, "invalid hosting")
		return
	}
	uid := id()
	if strings.TrimSpace(x.Code) == "" {
		x.Code = generatedSchoolCode(x.Name, uid)
	}
	_, e := s.db.Exec(r.Context(), `INSERT INTO schools(id,name,code,contact_name,contact_email,contact_phone,domain,hosting,notes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uid, x.Name, strings.ToUpper(strings.TrimSpace(x.Code)), x.ContactName, x.ContactEmail, x.ContactPhone, x.Domain, x.Hosting, x.Notes)
	if e != nil {
		fail(w, 409, "school code already exists or invalid data")
		return
	}
	s.audit(r.Context(), actor(r), "school.created", "school", uid)
	reply(w, 201, map[string]string{"id": uid})
}
func (s *Server) schoolStatus(w http.ResponseWriter, r *http.Request) {
	var x struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if body(r, &x) != nil {
		fail(w, 400, "invalid JSON")
		return
	}
	if x.Status != "pending" && x.Status != "active" && x.Status != "suspended" && x.Status != "archived" {
		fail(w, 400, "invalid status")
		return
	}
	tag, e := s.db.Exec(r.Context(), "UPDATE schools SET status=$1,updated_at=NOW() WHERE id=$2", x.Status, x.ID)
	if e != nil || tag.RowsAffected() == 0 {
		fail(w, 404, "school not found")
		return
	}
	s.audit(r.Context(), actor(r), "school.status."+x.Status, "school", x.ID)
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	fail(w, 405, "default plans are fixed; only the amount can be updated")
	return
	/*
		var x struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Currency    string   `json:"currency"`
			Interval    string   `json:"interval"`
			AmountMinor int64    `json:"amount_minor"`
			FeatureKeys []string `json:"feature_keys"`
		}
		if body(r, &x) != nil || !textOK(x.Name, 100) || x.AmountMinor < 0 {
			fail(w, 400, "invalid plan")
			return
		}
		if x.Currency == "" {
			x.Currency = "NGN"
		}
		if x.Interval != "term" {
			fail(w, 400, "billing interval must be term")
			return
		}
		uid := id()
		_, e := s.db.Exec(r.Context(), "INSERT INTO plans(id,name,description,amount_minor,currency,interval) VALUES($1,$2,$3,$4,$5,$6)", uid, x.Name, x.Description, x.AmountMinor, x.Currency, x.Interval)
		if e != nil {
			fail(w, 409, "plan already exists or invalid data")
			return
		}
		for _, featureKey := range x.FeatureKeys {
			featureKey = strings.TrimSpace(strings.ToLower(featureKey))
			if featureKey == "" {
				continue
			}
			featureID := id()
			if _, e = s.db.Exec(r.Context(), "INSERT INTO features(id,feature_key,name) VALUES($1,$2,$3) ON CONFLICT(feature_key) DO NOTHING", featureID, featureKey, strings.ReplaceAll(featureKey, "_", " ")); e != nil {
				fail(w, 400, "invalid feature")
				return
			}
			if e = s.db.QueryRow(r.Context(), "SELECT id FROM features WHERE feature_key=$1", featureKey).Scan(&featureID); e != nil {
				fail(w, 400, "invalid feature")
				return
			}
			if _, e = s.db.Exec(r.Context(), "INSERT INTO plan_features(plan_id,feature_id) VALUES($1,$2) ON CONFLICT DO NOTHING", uid, featureID); e != nil {
				fail(w, 400, "invalid feature")
				return
			}
		}
		s.audit(r.Context(), actor(r), "plan.created", "plan", uid)
		reply(w, 201, map[string]string{"id": uid})
	*/
}
func (s *Server) deletePlan(w http.ResponseWriter, r *http.Request) {
	planID := strings.TrimSpace(r.PathValue("id"))
	if planID == "" {
		fail(w, 400, "plan id is required")
		return
	}
	var subscriptions int
	if e := s.db.QueryRow(r.Context(), "SELECT COUNT(*) FROM subscriptions WHERE plan_id=$1", planID).Scan(&subscriptions); e != nil {
		fail(w, 500, "plan could not be checked")
		return
	}
	if subscriptions > 0 {
		fail(w, 409, "plan is assigned to a school and cannot be deleted")
		return
	}
	tag, e := s.db.Exec(r.Context(), "DELETE FROM plans WHERE id=$1", planID)
	if e != nil || tag.RowsAffected() == 0 {
		fail(w, 404, "plan not found")
		return
	}
	s.audit(r.Context(), actor(r), "plan.deleted", "plan", planID)
	reply(w, 200, map[string]any{"deleted": true, "id": planID})
}
func (s *Server) updatePlanAmount(w http.ResponseWriter, r *http.Request) {
	var x struct {
		ID          string `json:"id"`
		AmountMinor int64  `json:"amount_minor"`
	}
	if body(r, &x) != nil || strings.TrimSpace(x.ID) == "" || x.AmountMinor < 0 {
		fail(w, 400, "valid plan and amount are required")
		return
	}
	tag, e := s.db.Exec(r.Context(), "UPDATE plans SET amount_minor=$1 WHERE id=$2", x.AmountMinor, x.ID)
	if e != nil || tag.RowsAffected() == 0 {
		fail(w, 404, "plan not found")
		return
	}
	s.audit(r.Context(), actor(r), "plan.amount_updated", "plan", x.ID)
	reply(w, 200, map[string]any{"updated": true, "id": x.ID})
}
func (s *Server) createSubscription(w http.ResponseWriter, r *http.Request) {
	var x struct {
		SchoolID string   `json:"school_id"`
		PlanID   string   `json:"plan_id"`
		PlanIDs  []string `json:"plan_ids"`
		StartsOn string   `json:"starts_on"`
		EndsOn   string   `json:"ends_on"`
		Status   string   `json:"status"`
	}
	if body(r, &x) != nil {
		fail(w, 400, "invalid subscription")
		return
	}
	if len(x.PlanIDs) == 0 && x.PlanID != "" {
		x.PlanIDs = []string{x.PlanID}
	}
	if len(x.PlanIDs) == 0 {
		fail(w, 400, "at least one plan is required")
		return
	}
	if x.Status == "" {
		x.Status = "trial"
	}
	if _, e := time.Parse("2006-01-02", x.StartsOn); e != nil {
		fail(w, 400, "invalid start date")
		return
	}
	var end any
	if x.EndsOn != "" {
		if _, e := time.Parse("2006-01-02", x.EndsOn); e != nil || x.EndsOn < x.StartsOn {
			fail(w, 400, "invalid end date")
			return
		}
		end = x.EndsOn
	}
	uid := id()
	_, e := s.db.Exec(r.Context(), "INSERT INTO subscriptions(id,school_id,plan_id,starts_on,ends_on,status) VALUES($1,$2,$3,$4,$5,$6)", uid, x.SchoolID, x.PlanIDs[0], x.StartsOn, end, x.Status)
	if e != nil {
		fail(w, 400, "invalid subscription record")
		return
	}
	for _, planID := range x.PlanIDs {
		if _, e = s.db.Exec(r.Context(), "INSERT INTO subscription_plans(subscription_id,plan_id) VALUES($1,$2)", uid, planID); e != nil {
			fail(w, 400, "invalid plan selection")
			return
		}
	}
	s.audit(r.Context(), actor(r), "subscription.created", "subscription", uid)
	reply(w, 201, map[string]string{"id": uid})
}
func (s *Server) updateSubscription(w http.ResponseWriter, r *http.Request) {
	var x struct {
		ID, Status, StartsOn, EndsOn string
		SchoolName                   string   `json:"school_name"`
		PlanName                     string   `json:"plan_name"`
		SchoolID                     string   `json:"school_id"`
		CreatedAt                    string   `json:"created_at"`
		PlanIDs                      []string `json:"plan_ids"`
	}
	if body(r, &x) != nil || strings.TrimSpace(x.ID) == "" || len(x.PlanIDs) == 0 {
		fail(w, 400, "id and at least one plan are required")
		return
	}
	if _, e := time.Parse("2006-01-02", x.StartsOn); e != nil {
		fail(w, 400, "invalid start date")
		return
	}
	var end any
	if x.EndsOn != "" {
		if _, e := time.Parse("2006-01-02", x.EndsOn); e != nil || x.EndsOn < x.StartsOn {
			fail(w, 400, "invalid end date")
			return
		}
		end = x.EndsOn
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "subscription unavailable")
		return
	}
	defer tx.Rollback(r.Context())
	tag, e := tx.Exec(r.Context(), "UPDATE subscriptions SET plan_id=$1,starts_on=$2,ends_on=$3,status=$4 WHERE id=$5", x.PlanIDs[0], x.StartsOn, end, x.Status, x.ID)
	if e != nil || tag.RowsAffected() == 0 {
		fail(w, 404, "subscription not found")
		return
	}
	if _, e = tx.Exec(r.Context(), "DELETE FROM subscription_plans WHERE subscription_id=$1", x.ID); e != nil {
		fail(w, 500, "plans could not be updated")
		return
	}
	for _, planID := range x.PlanIDs {
		if _, e = tx.Exec(r.Context(), "INSERT INTO subscription_plans(subscription_id,plan_id) VALUES($1,$2)", x.ID, planID); e != nil {
			fail(w, 400, "invalid plan selection")
			return
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		fail(w, 500, "subscription could not be updated")
		return
	}
	s.audit(r.Context(), actor(r), "subscription.updated", "subscription", x.ID)
	reply(w, 200, map[string]any{"updated": true, "id": x.ID})
}
func (s *Server) createActivation(w http.ResponseWriter, r *http.Request) {
	var x struct {
		SubscriptionID string `json:"subscription_id"`
		TermName       string `json:"term_name"`
		StartsOn       string `json:"starts_on"`
		EndsOn         string `json:"ends_on"`
	}
	if body(r, &x) != nil || !textOK(x.TermName, 120) {
		fail(w, 400, "subscription and term name are required")
		return
	}
	if _, e := time.Parse("2006-01-02", x.StartsOn); e != nil {
		fail(w, 400, "invalid start date")
		return
	}
	if _, e := time.Parse("2006-01-02", x.EndsOn); e != nil || x.EndsOn < x.StartsOn {
		fail(w, 400, "invalid end date")
		return
	}
	var schoolID string
	if e := s.db.QueryRow(r.Context(), "SELECT school_id::text FROM subscriptions WHERE id=$1 AND status='active'", x.SubscriptionID).Scan(&schoolID); e != nil {
		fail(w, 404, "subscription not found")
		return
	}
	code := "EDU-" + strings.ToUpper(token()[:12])
	uid := id()
	ciphertext, e := encryptInvitation(code, s.invitationKey)
	if e != nil {
		fail(w, 500, "activation could not be created")
		return
	}
	tx, e := s.db.Begin(r.Context())
	if e != nil {
		fail(w, 500, "activation could not be created")
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "INSERT INTO term_activations(id,school_id,subscription_id,term_name,starts_on,ends_on,activation_code_hash,activation_code_ciphertext) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", uid, schoolID, x.SubscriptionID, x.TermName, x.StartsOn, x.EndsOn, digest(code), ciphertext); e != nil {
		fail(w, 409, "activation could not be created")
		return
	}
	var amount int64
	if e = tx.QueryRow(r.Context(), "SELECT COALESCE(SUM(p.amount_minor),0)::bigint FROM subscription_plans sp JOIN plans p ON p.id=sp.plan_id WHERE sp.subscription_id=$1", x.SubscriptionID).Scan(&amount); e != nil {
		fail(w, 500, "invoice could not be calculated")
		return
	}
	invoiceID := id()
	invoiceNo := "INV-" + strings.ToUpper(strings.ReplaceAll(invoiceID[:13], "-", ""))
	if _, e = tx.Exec(r.Context(), `INSERT INTO invoices(id,invoice_no,school_id,subscription_id,activation_id,description,amount_minor,currency,status,due_on,issued_on) VALUES($1,$2,$3,$4,$5,$6,$7,'NGN','issued',$8,CURRENT_DATE)`, invoiceID, invoiceNo, schoolID, x.SubscriptionID, uid, "Subscription · "+x.TermName, amount, x.EndsOn); e != nil {
		fail(w, 500, "invoice could not be created")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		fail(w, 500, "activation could not be committed")
		return
	}
	s.audit(r.Context(), actor(r), "term_activation.created", "term_activation", uid)
	reply(w, 201, map[string]any{"id": uid, "school_id": schoolID, "subscription_id": x.SubscriptionID, "term_name": x.TermName, "starts_on": x.StartsOn, "ends_on": x.EndsOn, "activation_code": code, "invoice_id": invoiceID, "invoice_no": invoiceNo, "amount_minor": amount})
}
func (s *Server) verifyActivation(w http.ResponseWriter, r *http.Request) {
	var x struct {
		SchoolID       string `json:"school_id"`
		ActivationCode string `json:"activation_code"`
		InstanceID     string `json:"instance_id"`
	}
	if body(r, &x) != nil || !textOK(x.SchoolID, 80) || !textOK(x.ActivationCode, 80) || !textOK(x.InstanceID, 160) {
		fail(w, 400, "school_id, activation_code and instance_id are required")
		return
	}
	var activationID, subscriptionID, termName, startsOn, endsOn, status, subscriptionStatus string
	e := s.db.QueryRow(r.Context(), `SELECT a.id::text,a.subscription_id::text,a.term_name,a.starts_on::text,a.ends_on::text,a.status,s.status FROM term_activations a JOIN subscriptions s ON s.id=a.subscription_id WHERE a.school_id=$1 AND a.activation_code_hash=$2`, x.SchoolID, digest(x.ActivationCode)).Scan(&activationID, &subscriptionID, &termName, &startsOn, &endsOn, &status, &subscriptionStatus)
	if e != nil || status == "revoked" {
		fail(w, 401, "invalid activation code")
		return
	}
	if status == "expired" || endsOn < time.Now().Format("2006-01-02") {
		fail(w, 409, "activation has expired")
		return
	}
	if subscriptionStatus != "active" && subscriptionStatus != "trial" {
		fail(w, 409, "subscription is inactive")
		return
	}
	_, _ = s.db.Exec(r.Context(), "UPDATE term_activations SET status='active',activated_at=COALESCE(activated_at,NOW()) WHERE id=$1", activationID)
	rows, e := s.db.Query(r.Context(), `SELECT DISTINCT f.feature_key FROM subscription_plans sp JOIN plan_features pf ON pf.plan_id=sp.plan_id JOIN features f ON f.id=pf.feature_id WHERE sp.subscription_id=$1 ORDER BY f.feature_key`, subscriptionID)
	if e != nil {
		fail(w, 500, "features unavailable")
		return
	}
	defer rows.Close()
	features := []string{}
	for rows.Next() {
		var feature string
		if rows.Scan(&feature) == nil {
			features = append(features, feature)
		}
	}
	reply(w, 200, map[string]any{"valid": true, "activation_id": activationID, "school_id": x.SchoolID, "subscription_id": subscriptionID, "term_name": termName, "starts_on": startsOn, "ends_on": endsOn, "features": features, "instance_id": x.InstanceID})
}
func (s *Server) emailActivation(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var code, termName, schoolName, email string
	if e := s.db.QueryRow(r.Context(), `SELECT a.term_name,sc.name,sc.contact_email FROM term_activations a JOIN schools sc ON sc.id=a.school_id WHERE a.id=$1`, id).Scan(&termName, &schoolName, &email); e != nil {
		fail(w, 404, "activation not found")
		return
	}
	if strings.TrimSpace(email) == "" {
		fail(w, 400, "school email is unavailable")
		return
	}
	// The plaintext code is intentionally supplied by the caller only when the activation is created.
	var x struct {
		Code string `json:"activation_code"`
	}
	if body(r, &x) != nil || !textOK(x.Code, 80) {
		fail(w, 400, "activation code is required")
		return
	}
	code = x.Code
	html := fmt.Sprintf("<p>Hello %s,</p><p>Your EduCraft360 activation code for <strong>%s</strong> is:</p><p style=\"font-size:24px;font-weight:700;letter-spacing:2px\">%s</p><p>Use this code in your school installation.</p>", htmlEscape(schoolName), htmlEscape(termName), htmlEscape(code))
	if e := s.sendEmail(email, "EduCraft360 activation code", html); e != nil {
		fail(w, 503, e.Error())
		return
	}
	reply(w, 200, map[string]any{"sent": true, "to": email})
}
func (s *Server) activationCode(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var ciphertext, status string
	if e := s.db.QueryRow(r.Context(), "SELECT activation_code_ciphertext,status FROM term_activations WHERE id=$1", id).Scan(&ciphertext, &status); e != nil {
		fail(w, 404, "activation not found")
		return
	}
	if status == "revoked" {
		fail(w, 409, "activation has been revoked")
		return
	}
	code, e := decryptInvitation(ciphertext, s.invitationKey)
	if e != nil {
		fail(w, 409, "activation code unavailable")
		return
	}
	reply(w, 200, map[string]string{"activation_code": code})
}
func (s *Server) createInvoice(w http.ResponseWriter, r *http.Request) {
	var x struct {
		SchoolID         string `json:"school_id"`
		SubscriptionID   string `json:"subscription_id"`
		Description      string `json:"description"`
		Currency         string `json:"currency"`
		DueOn            string `json:"due_on"`
		AmountMinor      int64  `json:"amount_minor"`
		UnitCount        int64  `json:"unit_count"`
		UnitAmountMinor  int64  `json:"unit_amount_minor"`
		AmountPaidMinor  int64  `json:"amount_paid_minor"`
		PaymentReference string `json:"payment_reference"`
	}
	if body(r, &x) != nil || !textOK(x.Description, 500) || x.AmountMinor < 0 || x.UnitCount < 0 || x.UnitAmountMinor < 0 || x.AmountPaidMinor < 0 {
		fail(w, 400, "invalid invoice")
		return
	}
	if _, e := time.Parse("2006-01-02", x.DueOn); e != nil {
		fail(w, 400, "invalid due date")
		return
	}
	if x.Currency == "" {
		x.Currency = "NGN"
	}
	if x.UnitCount > 0 {
		if x.UnitAmountMinor <= 0 {
			fail(w, 400, "unit amount is required when units are provided")
			return
		}
		x.AmountMinor = x.UnitCount * x.UnitAmountMinor
	}
	if x.AmountPaidMinor > x.AmountMinor {
		fail(w, 400, "amount paid cannot exceed invoice total")
		return
	}
	if x.PaymentReference != "" && !textOK(x.PaymentReference, 120) {
		fail(w, 400, "invalid payment reference")
		return
	}
	var sub any
	if x.SubscriptionID != "" {
		sub = x.SubscriptionID
	}
	uid := id()
	number := "INV-" + strings.ToUpper(strings.ReplaceAll(uid[:13], "-", ""))
	if x.AmountPaidMinor > 0 && x.PaymentReference == "" {
		x.PaymentReference = "AUTO-" + strings.ToUpper(strings.ReplaceAll(uid[:13], "-", ""))
	}
	tag, e := s.db.Exec(r.Context(), "INSERT INTO invoices(id,invoice_no,school_id,subscription_id,description,unit_count,unit_amount_minor,amount_minor,currency,status,due_on,issued_on) SELECT $1::uuid,$2::text,$3::uuid,$4::uuid,$5::text,$6::bigint,$7::bigint,$8::bigint,$9::text,CASE WHEN $11::bigint > 0 AND $11::bigint = $8::bigint THEN 'paid' WHEN $11::bigint > 0 THEN 'part_paid' ELSE 'issued' END,$10::date,CURRENT_DATE WHERE $4::uuid IS NULL OR EXISTS (SELECT 1 FROM subscriptions WHERE id=$4 AND school_id=$3)", uid, number, x.SchoolID, sub, x.Description, x.UnitCount, x.UnitAmountMinor, x.AmountMinor, x.Currency, x.DueOn, x.AmountPaidMinor)
	if e != nil || tag.RowsAffected() == 0 {
		fail(w, 400, "invalid invoice data")
		return
	}
	if x.AmountPaidMinor > 0 {
		if _, e = s.db.Exec(r.Context(), "INSERT INTO payments(id,invoice_id,amount_minor,reference,paid_on,recorded_by) VALUES($1,$2,$3,$4,CURRENT_DATE,$5)", id(), uid, x.AmountPaidMinor, x.PaymentReference, actor(r).ID); e != nil {
			fail(w, 409, "initial payment could not be recorded")
			return
		}
	}
	s.audit(r.Context(), actor(r), "invoice.created", "invoice", uid)
	reply(w, 201, map[string]any{"id": uid, "invoice_no": number, "amount_minor": x.AmountMinor, "amount_paid_minor": x.AmountPaidMinor})
}
func (s *Server) invoiceIssue(w http.ResponseWriter, r *http.Request) {
	var x struct {
		ID string `json:"id"`
	}
	if body(r, &x) != nil {
		fail(w, 400, "invalid request")
		return
	}
	tag, e := s.db.Exec(r.Context(), "UPDATE invoices SET status='issued',issued_on=CURRENT_DATE WHERE id=$1 AND status='draft'", x.ID)
	if e != nil || tag.RowsAffected() == 0 {
		fail(w, 409, "only draft invoices may be issued")
		return
	}
	s.audit(r.Context(), actor(r), "invoice.issued", "invoice", x.ID)
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) listInvoicePayments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rows, e := s.db.Query(r.Context(), `SELECT id::text,amount_minor,reference,student_number,paid_on::text,created_at FROM payments WHERE invoice_id=$1 ORDER BY paid_on ASC,created_at ASC`, id)
	if e != nil {
		fail(w, 500, "payments unavailable")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var paymentID, reference, studentNumber, paidOn string
		var amount int64
		var createdAt time.Time
		if e = rows.Scan(&paymentID, &amount, &reference, &studentNumber, &paidOn, &createdAt); e != nil {
			fail(w, 500, "payments unavailable")
			return
		}
		result = append(result, map[string]any{"id": paymentID, "amount_minor": amount, "reference": reference, "student_number": studentNumber, "paid_on": paidOn, "created_at": createdAt})
	}
	reply(w, 200, result)
}
func (s *Server) schoolSubscription(w http.ResponseWriter, r *http.Request) {
	instance := schoolInstanceFrom(r)
	var schoolCode string
	if e := s.db.QueryRow(r.Context(), "SELECT code FROM schools WHERE id=$1", instance.SchoolID).Scan(&schoolCode); e != nil {
		fail(w, 404, "school not found")
		return
	}
	rows, e := s.db.Query(r.Context(), `SELECT s.id::text,s.status,s.starts_on::text,s.ends_on::text,COALESCE(p.code,''),COALESCE(p.name,''),COALESCE(f.feature_key,'')
		FROM subscriptions s LEFT JOIN subscription_plans sp ON sp.subscription_id=s.id LEFT JOIN plans p ON p.id=sp.plan_id
		LEFT JOIN plan_features pf ON pf.plan_id=p.id LEFT JOIN features f ON f.id=pf.feature_id
		WHERE s.school_id=$1 AND s.status IN ('active','trial') AND s.ends_on>=CURRENT_DATE ORDER BY s.ends_on DESC`, instance.SchoolID)
	if e != nil {
		fail(w, 500, "subscription unavailable")
		return
	}
	defer rows.Close()
	type subscription struct {
		ID, Status, StartsOn, EndsOn string
		Plans                        map[string]string
		Features                     map[string]bool
	}
	items := map[string]*subscription{}
	for rows.Next() {
		var id, status, startsOn, endsOn, planCode, planName, feature string
		if e = rows.Scan(&id, &status, &startsOn, &endsOn, &planCode, &planName, &feature); e != nil {
			fail(w, 500, "subscription unavailable")
			return
		}
		item := items[id]
		if item == nil {
			item = &subscription{ID: id, Status: status, StartsOn: startsOn, EndsOn: endsOn, Plans: map[string]string{}, Features: map[string]bool{}}
			items[id] = item
		}
		if planCode != "" {
			item.Plans[planCode] = planName
		}
		if feature != "" {
			item.Features[feature] = true
		}
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		plans := make([]map[string]string, 0, len(item.Plans))
		for code, name := range item.Plans {
			plans = append(plans, map[string]string{"code": code, "name": name})
		}
		features := make([]string, 0, len(item.Features))
		for feature := range item.Features {
			features = append(features, feature)
		}
		result = append(result, map[string]any{"id": item.ID, "status": item.Status, "starts_on": item.StartsOn, "ends_on": item.EndsOn, "plans": plans, "features": features})
	}
	reply(w, 200, map[string]any{"school_id": instance.SchoolID, "school_code": schoolCode, "instance_id": instance.InstanceID, "subscriptions": result})
}
func (s *Server) createSchoolTicket(w http.ResponseWriter, r *http.Request) {
	instance := schoolInstanceFrom(r)
	var x struct{ Subject, Description, Category, Priority, RequesterName, RequesterEmail string }
	if body(r, &x) != nil || !textOK(x.Subject, 180) || !textOK(x.Description, 8000) {
		fail(w, 400, "subject and description required")
		return
	}
	if x.Category == "" {
		x.Category = "general"
	}
	if x.Priority == "" {
		x.Priority = "normal"
	}
	if x.Priority != "low" && x.Priority != "normal" && x.Priority != "high" && x.Priority != "urgent" {
		fail(w, 400, "invalid priority")
		return
	}
	uid := id()
	number := "SUP-" + strings.ToUpper(strings.ReplaceAll(uid[:13], "-", ""))
	_, e := s.db.Exec(r.Context(), `INSERT INTO tickets(id,ticket_no,school_id,subject,description,category,priority,requester_name,requester_email) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uid, number, instance.SchoolID, x.Subject, x.Description, x.Category, x.Priority, x.RequesterName, x.RequesterEmail)
	if e != nil {
		fail(w, 400, "ticket could not be created")
		return
	}
	reply(w, 201, map[string]any{"id": uid, "ticket_no": number, "school_id": instance.SchoolID, "school_code": ""})
}
func (s *Server) listSchoolTickets(w http.ResponseWriter, r *http.Request) {
	instance := schoolInstanceFrom(r)
	rows, e := s.db.Query(r.Context(), `SELECT id::text,ticket_no,subject,category,priority,status,created_at,updated_at FROM tickets WHERE school_id=$1 ORDER BY created_at DESC`, instance.SchoolID)
	if e != nil {
		fail(w, 500, "tickets unavailable")
		return
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, number, subject, category, priority, status string
		var created, updated time.Time
		if e = rows.Scan(&id, &number, &subject, &category, &priority, &status, &created, &updated); e != nil {
			fail(w, 500, "tickets unavailable")
			return
		}
		result = append(result, map[string]any{"id": id, "ticket_no": number, "subject": subject, "category": category, "priority": priority, "status": status, "created_at": created, "updated_at": updated})
	}
	reply(w, 200, result)
}
func (s *Server) schoolTicketDetail(w http.ResponseWriter, r *http.Request) {
	instance := schoolInstanceFrom(r)
	id := r.PathValue("id")
	var out struct {
		ID, TicketNo, Subject, Description, Category, Priority, Status, RequesterName, RequesterEmail string
		CreatedAt                                                                                     time.Time
		Replies                                                                                       []map[string]any
	}
	e := s.db.QueryRow(r.Context(), `SELECT id::text,ticket_no,subject,description,category,priority,status,requester_name,requester_email,created_at FROM tickets WHERE id=$1 AND school_id=$2`, id, instance.SchoolID).Scan(&out.ID, &out.TicketNo, &out.Subject, &out.Description, &out.Category, &out.Priority, &out.Status, &out.RequesterName, &out.RequesterEmail, &out.CreatedAt)
	if e != nil {
		fail(w, 404, "ticket not found")
		return
	}
	rows, e := s.db.Query(r.Context(), `SELECT id::text,'School'::text,body,false,created_at FROM school_ticket_replies WHERE ticket_id=$1 UNION ALL SELECT tr.id::text,o.name,tr.body,tr.internal,tr.created_at FROM ticket_replies tr JOIN operators o ON o.id=tr.author_id WHERE tr.ticket_id=$1 AND NOT tr.internal ORDER BY created_at`, id)
	if e != nil {
		fail(w, 500, "replies unavailable")
		return
	}
	defer rows.Close()
	out.Replies = []map[string]any{}
	for rows.Next() {
		var rid, author, body string
		var internal bool
		var created time.Time
		if rows.Scan(&rid, &author, &body, &internal, &created) == nil {
			out.Replies = append(out.Replies, map[string]any{"id": rid, "author": author, "body": body, "internal": internal, "created_at": created})
		}
	}
	reply(w, 200, out)
}
func (s *Server) schoolTicketReply(w http.ResponseWriter, r *http.Request) {
	instance := schoolInstanceFrom(r)
	ticketID := r.PathValue("id")
	var x struct {
		Body string `json:"body"`
	}
	if body(r, &x) != nil || !textOK(x.Body, 8000) {
		fail(w, 400, "reply required")
		return
	}
	var exists bool
	if e := s.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM tickets WHERE id=$1 AND school_id=$2)", ticketID, instance.SchoolID).Scan(&exists); e != nil || !exists {
		fail(w, 404, "ticket not found")
		return
	}
	rid := id()
	if _, e := s.db.Exec(r.Context(), "INSERT INTO school_ticket_replies(id,ticket_id,instance_id,body) VALUES($1,$2,$3,$4)", rid, ticketID, instance.InstanceID, x.Body); e != nil {
		fail(w, 400, "reply could not be added")
		return
	}
	_, _ = s.db.Exec(r.Context(), "UPDATE tickets SET status='open',updated_at=NOW() WHERE id=$1 AND status IN ('awaiting_customer','resolved')", ticketID)
	reply(w, 201, map[string]string{"id": rid})
}
func (s *Server) revokeSchoolCredential(w http.ResponseWriter, r *http.Request) {
	credentialID := strings.TrimSpace(r.PathValue("id"))
	if credentialID == "" {
		fail(w, 400, "credential id is required")
		return
	}
	tag, e := s.db.Exec(r.Context(), "UPDATE school_instance_credentials SET revoked_at=COALESCE(revoked_at,NOW()) WHERE id=$1", credentialID)
	if e != nil {
		fail(w, 500, "credential could not be revoked")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "credential not found")
		return
	}
	s.audit(r.Context(), actor(r), "school.credential_revoked", "school_instance_credential", credentialID)
	reply(w, 200, map[string]any{"revoked": true, "id": credentialID})
}
func (s *Server) rotateSchoolCredential(w http.ResponseWriter, r *http.Request) {
	var x struct{ SchoolID, InstanceID string }
	if body(r, &x) != nil || !textOK(x.SchoolID, 80) || !textOK(x.InstanceID, 160) {
		fail(w, 400, "school_id and instance_id required")
		return
	}
	if _, e := s.db.Exec(r.Context(), "UPDATE school_instance_credentials SET revoked_at=COALESCE(revoked_at,NOW()) WHERE school_id=$1 AND instance_id=$2 AND revoked_at IS NULL", x.SchoolID, x.InstanceID); e != nil {
		fail(w, 500, "credential rotation unavailable")
		return
	}
	apiToken := token()
	credentialID := id()
	expires := time.Now().AddDate(1, 0, 0)
	if _, e := s.db.Exec(r.Context(), "INSERT INTO school_instance_credentials(id,school_id,instance_id,token_hash,expires_at) VALUES($1,$2,$3,$4,$5)", credentialID, x.SchoolID, x.InstanceID, digest(apiToken), expires); e != nil {
		fail(w, 500, "replacement credential could not be issued")
		return
	}
	s.audit(r.Context(), actor(r), "school.credential_rotated", "school_instance_credential", credentialID)
	reply(w, 200, map[string]any{"rotated": true, "credential_id": credentialID, "school_id": x.SchoolID, "instance_id": x.InstanceID, "api_token": apiToken, "api_token_expires_at": expires})
}
func (s *Server) recordPayment(w http.ResponseWriter, r *http.Request) {
	var x struct {
		InvoiceID     string `json:"invoice_id"`
		Reference     string `json:"reference"`
		StudentNumber string `json:"student_number"`
		UnitCount     int64  `json:"unit_count"`
		PaidOn        string `json:"paid_on"`
		AmountMinor   int64  `json:"amount_minor"`
	}
	if body(r, &x) != nil || !textOK(x.Reference, 120) || !textOK(x.StudentNumber, 120) || x.UnitCount < 0 || x.AmountMinor <= 0 {
		fail(w, 400, "invalid payment")
		return
	}
	if _, e := time.Parse("2006-01-02", x.PaidOn); e != nil {
		fail(w, 400, "invalid date")
		return
	}
	ctx := r.Context()
	tx, e := s.db.Begin(ctx)
	if e != nil {
		fail(w, 500, "database unavailable")
		return
	}
	defer tx.Rollback(ctx)
	var total, unitAmount, existingUnits int64
	var status string
	e = tx.QueryRow(ctx, "SELECT amount_minor,status,unit_amount_minor,unit_count FROM invoices WHERE id=$1 FOR UPDATE", x.InvoiceID).Scan(&total, &status, &unitAmount, &existingUnits)
	if e != nil || status == "draft" || status == "void" {
		fail(w, 409, "invoice must be issued")
		return
	}
	if unitAmount <= 0 {
		unitAmount = total
	}
	if existingUnits > 0 {
		if x.UnitCount > 0 && x.UnitCount != existingUnits {
			fail(w, 409, "student units are already set for this invoice")
			return
		}
		x.UnitCount = existingUnits
	} else if x.UnitCount <= 0 {
		fail(w, 400, "student units are required for the first payment")
		return
	}
	total = x.UnitCount * unitAmount
	if existingUnits <= 0 {
		if _, e = tx.Exec(ctx, "UPDATE invoices SET unit_count=$1,unit_amount_minor=$2 WHERE id=$3", x.UnitCount, unitAmount, x.InvoiceID); e != nil {
			fail(w, 500, "invoice could not be completed")
			return
		}
	}
	var paid int64
	e = tx.QueryRow(ctx, "SELECT COALESCE(SUM(amount_minor),0)::bigint FROM payments WHERE invoice_id=$1", x.InvoiceID).Scan(&paid)
	if e != nil || x.AmountMinor > total-paid {
		fail(w, 409, "payment exceeds outstanding invoice amount")
		return
	}
	x.Reference = strings.TrimSpace(x.Reference)
	uid := id()
	_, e = tx.Exec(ctx, "INSERT INTO payments(id,invoice_id,amount_minor,reference,student_number,paid_on,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7)", uid, x.InvoiceID, x.AmountMinor, x.Reference, x.StudentNumber, x.PaidOn, actor(r).ID)
	if e != nil {
		fail(w, 409, "payment reference already exists")
		return
	}
	newStatus := "part_paid"
	if paid+x.AmountMinor == total {
		newStatus = "paid"
	}
	_, e = tx.Exec(ctx, "UPDATE invoices SET status=$1 WHERE id=$2", newStatus, x.InvoiceID)
	if e != nil || tx.Commit(ctx) != nil {
		fail(w, 500, "payment could not be committed")
		return
	}
	s.audit(ctx, actor(r), "payment.recorded", "invoice", x.InvoiceID)
	reply(w, 201, map[string]string{"id": uid})
}
func (s *Server) createTicket(w http.ResponseWriter, r *http.Request) {
	var x struct {
		SchoolID       string `json:"school_id"`
		SchoolCode     string `json:"school_code"`
		Subject        string `json:"subject"`
		Description    string `json:"description"`
		Category       string `json:"category"`
		Priority       string `json:"priority"`
		RequesterName  string `json:"requester_name"`
		RequesterEmail string `json:"requester_email"`
	}
	if body(r, &x) != nil || !textOK(x.Subject, 180) || !textOK(x.Description, 8000) {
		fail(w, 400, "subject and description required")
		return
	}
	if x.SchoolID == "" && x.SchoolCode != "" {
		if e := s.db.QueryRow(r.Context(), "SELECT id::text FROM schools WHERE code=$1", strings.TrimSpace(x.SchoolCode)).Scan(&x.SchoolID); e != nil {
			fail(w, 404, "school code not found")
			return
		}
	}
	if x.Priority == "" {
		x.Priority = "normal"
	}
	if x.Priority != "low" && x.Priority != "normal" && x.Priority != "high" && x.Priority != "urgent" {
		fail(w, 400, "invalid priority")
		return
	}
	if x.Category == "" {
		x.Category = "general"
	}
	uid := id()
	number := "SUP-" + strings.ToUpper(strings.ReplaceAll(uid[:13], "-", ""))
	_, e := s.db.Exec(r.Context(), `INSERT INTO tickets(id,ticket_no,school_id,subject,description,category,priority,requester_name,requester_email) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uid, number, x.SchoolID, x.Subject, x.Description, x.Category, x.Priority, x.RequesterName, x.RequesterEmail)
	if e != nil {
		fail(w, 400, "invalid school or ticket data")
		return
	}
	s.audit(r.Context(), actor(r), "ticket.created", "ticket", uid)
	reply(w, 201, map[string]string{"id": uid, "ticket_no": number})
}
func canManageTickets(role string) bool {
	return role == "owner" || role == "admin" || role == "support"
}

func (s *Server) ticketAssignees(w http.ResponseWriter, r *http.Request) {
	s.list(w, r, "ticket_assignees")
}

func (s *Server) ticketDetail(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("id")
	var out struct {
		ID, TicketNo, School, SchoolCode, Subject, Description, Category, Priority, Status, AssignedTo, RequesterName, RequesterEmail string
		CreatedAt                                                                                                                     time.Time
		Replies                                                                                                                       []map[string]any
	}
	err := s.db.QueryRow(r.Context(), `SELECT t.id::text,t.ticket_no,s.name,s.code,t.subject,t.description,t.category,t.priority,t.status,COALESCE(t.assigned_to::text,''),t.requester_name,t.requester_email,t.created_at FROM tickets t JOIN schools s ON t.school_id=s.id WHERE t.id=$1`, uid).Scan(&out.ID, &out.TicketNo, &out.School, &out.SchoolCode, &out.Subject, &out.Description, &out.Category, &out.Priority, &out.Status, &out.AssignedTo, &out.RequesterName, &out.RequesterEmail, &out.CreatedAt)
	if err != nil {
		fail(w, 404, "ticket not found")
		return
	}
	rows, err := s.db.Query(r.Context(), "SELECT tr.id::text,o.name,tr.body,tr.internal,tr.created_at FROM ticket_replies tr JOIN operators o ON tr.author_id=o.id WHERE tr.ticket_id=$1 AND (NOT tr.internal OR $2) UNION ALL SELECT sr.id::text,'School',sr.body,false,sr.created_at FROM school_ticket_replies sr WHERE sr.ticket_id=$1 ORDER BY created_at", uid, canManageTickets(actor(r).Role))
	if err != nil {
		fail(w, 500, "replies unavailable")
		return
	}
	defer rows.Close()
	out.Replies = []map[string]any{}
	for rows.Next() {
		var id, name, b string
		var internal bool
		var created time.Time
		if rows.Scan(&id, &name, &b, &internal, &created) == nil {
			out.Replies = append(out.Replies, map[string]any{"id": id, "author": name, "body": b, "internal": internal, "created_at": created.Format("2 Jan 2006, 15:04")})
		}
	}
	reply(w, 200, out)
}
func (s *Server) ticketUpdate(w http.ResponseWriter, r *http.Request) {
	var x struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		AssignedTo string `json:"assigned_to"`
	}
	if body(r, &x) != nil {
		fail(w, 400, "invalid request")
		return
	}
	allowed := map[string]bool{"open": true, "assigned": true, "in_progress": true, "awaiting_customer": true, "resolved": true, "closed": true}
	if !allowed[x.Status] {
		fail(w, 400, "invalid status")
		return
	}
	var assigned any
	if x.AssignedTo != "" {
		assigned = x.AssignedTo
	}
	tag, e := s.db.Exec(r.Context(), "UPDATE tickets SET status=$1,assigned_to=$2,updated_at=NOW() WHERE id=$3 AND ($2::uuid IS NULL OR assigned_to=$2 OR EXISTS (SELECT 1 FROM operators WHERE id=$2 AND active AND role IN ('owner','admin','support')))", x.Status, assigned, x.ID)
	if e != nil || tag.RowsAffected() == 0 {
		fail(w, 400, "ticket not found or invalid assignee")
		return
	}
	s.audit(r.Context(), actor(r), "ticket.updated", "ticket", x.ID)
	reply(w, 200, map[string]bool{"ok": true})
}
func (s *Server) ticketReply(w http.ResponseWriter, r *http.Request) {
	var x struct {
		TicketID string `json:"ticket_id"`
		Body     string `json:"body"`
		Internal bool   `json:"internal"`
	}
	if body(r, &x) != nil || !textOK(x.Body, 8000) {
		fail(w, 400, "reply required")
		return
	}
	uid := id()
	tag, e := s.db.Exec(r.Context(), `INSERT INTO ticket_replies(id,ticket_id,author_id,body,internal) VALUES($1,$2,$3,$4,$5)`, uid, x.TicketID, actor(r).ID, x.Body, x.Internal)
	if e != nil || tag.RowsAffected() == 0 {
		fail(w, 400, "ticket not found")
		return
	}
	s.audit(r.Context(), actor(r), "ticket.replied", "ticket", x.TicketID)
	reply(w, 201, map[string]string{"id": uid})
}
func (s *Server) createOperator(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if body(r, &x) != nil || !textOK(x.Name, 120) || !strings.Contains(x.Email, "@") || (x.Password != "" && len(x.Password) < 12) {
		fail(w, 400, "name and a valid email required; password must be 12+ characters when provided")
		return
	}
	x.Email = strings.ToLower(strings.TrimSpace(x.Email))
	if x.Password == "" {
		x.Password = token()[:16]
	}
	allowed := map[string]bool{"owner": true, "admin": true, "finance": true, "support": true, "viewer": true}
	if !allowed[x.Role] || (x.Role == "owner" && actor(r).Role != "owner") {
		fail(w, 403, "role not permitted")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(x.Password), bcrypt.DefaultCost)
	if e != nil {
		fail(w, 400, "invalid password")
		return
	}
	uid := id()
	_, e = s.db.Exec(r.Context(), "INSERT INTO operators(id,name,email,password_hash,role) VALUES($1,$2,$3,$4,$5)", uid, x.Name, x.Email, string(hash), x.Role)
	if e != nil {
		fail(w, 409, "email already exists")
		return
	}
	emailHTML := fmt.Sprintf("<p>Hello %s,</p><p>Your EduCraft360 operator account has been created.</p><p><strong>Email:</strong> %s<br><strong>Role:</strong> %s<br><strong>Temporary password:</strong> %s</p><p>Please sign in and change your password immediately.</p>", x.Name, x.Email, x.Role, x.Password)
	if e = s.sendEmail(x.Email, "Your EduCraft360 operator account", emailHTML); e != nil {
		fail(w, 502, "operator created but credentials email could not be sent")
		return
	}
	s.audit(r.Context(), actor(r), "operator.created", "operator", uid)
	reply(w, 201, map[string]string{"id": uid})
}
func migrate(db *pgxpool.Pool) error {
	b, e := os.ReadFile("migrations/001_init.sql")
	if e != nil {
		return e
	}
	for _, statement := range strings.Split(string(b), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, e = db.Exec(context.Background(), statement); e != nil {
			return e
		}
	}
	return nil
}
func seed(db *pgxpool.Pool) error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL")))
	pw := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	name := os.Getenv("BOOTSTRAP_ADMIN_NAME")
	if name == "" {
		name = "Platform Owner"
	}
	if !strings.Contains(email, "@") || len(pw) < 12 {
		return errors.New("set BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD (12+ characters)")
	}
	var count int
	err := db.QueryRow(context.Background(), "SELECT COUNT(*) FROM operators").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("operators already exist; refusing to bootstrap again")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(context.Background(), "INSERT INTO operators(id,name,email,password_hash,role) VALUES($1,$2,$3,$4,'owner')", id(), name, email, string(hash))
	return err
}
func resetBootstrapPassword(db *pgxpool.Pool) error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL")))
	pw := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if !strings.Contains(email, "@") || len(pw) < 12 {
		return errors.New("set BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD (12+ characters)")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tag, err := db.Exec(context.Background(), "UPDATE operators SET password_hash=$1,active=TRUE WHERE email=$2", string(hash), email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("operator email not found")
	}
	return nil
}
func main() {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL required")
	}
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	if e = db.Ping(ctx); e != nil {
		log.Fatal(e)
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			e = migrate(db)
		case "bootstrap":
			e = seed(db)
		case "reset-password":
			e = resetBootstrapPassword(db)
		default:
			e = fmt.Errorf("unknown command %s", os.Args[1])
		}
		if e != nil {
			log.Fatal(e)
		}
		fmt.Println("completed")
		return
	}
	s := &Server{db: db, secure: os.Getenv("COOKIE_SECURE") == "true", allowedOrigin: os.Getenv("APP_ORIGIN"), invitationKey: os.Getenv("INVITATION_SIGNING_KEY"), resendAPIKey: os.Getenv("RESEND_API_KEY"), resendFrom: os.Getenv("RESEND_FROM_EMAIL")}
	if s.allowedOrigin == "" {
		s.allowedOrigin = "http://localhost:3000"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /auth/login", s.origin(s.login))
	mux.HandleFunc("POST /auth/logout", s.origin(s.logout))
	mux.HandleFunc("POST /auth/password", s.origin(s.guard(s.changePassword)))
	mux.HandleFunc("GET /auth/me", s.guard(s.me))
	mux.HandleFunc("GET /dashboard", s.guard(s.dashboard))
	mux.HandleFunc("GET /installation-invitations", s.guard(s.listInvitations, "owner", "admin"))
	mux.HandleFunc("GET /installation-invitations/{id}/link", s.guard(s.invitationLink, "owner", "admin"))
	mux.HandleFunc("POST /installation-invitations/{id}/email", s.origin(s.guard(s.emailInvitation, "owner", "admin")))
	mux.HandleFunc("GET /invitations", s.guard(s.listInvitations, "owner", "admin"))
	mux.HandleFunc("POST /installation-invitations/verify", s.origin(s.verifyInvitation))
	mux.HandleFunc("POST /installation-invitations/consume", s.origin(s.consumeInvitation))
	mux.HandleFunc("GET /school/subscription", s.schoolGuard(s.schoolSubscription))
	mux.HandleFunc("POST /school/tickets", s.schoolGuard(s.createSchoolTicket))
	mux.HandleFunc("GET /school/tickets", s.schoolGuard(s.listSchoolTickets))
	mux.HandleFunc("GET /school/tickets/{id}", s.schoolGuard(s.schoolTicketDetail))
	mux.HandleFunc("POST /school/tickets/{id}/replies", s.schoolGuard(s.schoolTicketReply))
	mux.HandleFunc("DELETE /school-instance-credentials/{id}", s.origin(s.guard(s.revokeSchoolCredential, "owner", "admin")))
	mux.HandleFunc("POST /school-instance-credentials/rotate", s.origin(s.guard(s.rotateSchoolCredential, "owner", "admin")))
	mux.HandleFunc("DELETE /installation-invitations/{id}", s.origin(s.guard(s.revokeInvitation, "owner", "admin")))
	for _, kind := range []string{"schools", "plans", "subscriptions", "invoices", "tickets", "operators"} {
		k := kind
		roles := []string{}
		switch k {
		case "operators":
			roles = []string{"owner", "admin"}
		case "plans", "subscriptions", "invoices":
			roles = []string{"owner", "admin", "finance"}
		case "tickets":
			roles = []string{"owner", "admin", "support"}
		case "schools":
			roles = []string{"owner", "admin", "finance", "support"}
		}
		mux.HandleFunc("GET /"+k, s.guard(func(w http.ResponseWriter, r *http.Request) { s.list(w, r, k) }, roles...))
	}
	mux.HandleFunc("POST /schools", s.origin(s.guard(s.createSchool, "owner", "admin")))
	mux.HandleFunc("POST /installation-invitations", s.origin(s.guard(s.createInvitation, "owner", "admin")))
	mux.HandleFunc("PATCH /schools/status", s.origin(s.guard(s.schoolStatus, "owner", "admin")))
	mux.HandleFunc("POST /plans", s.origin(s.guard(s.createPlan, "owner", "admin", "finance")))
	mux.HandleFunc("DELETE /plans/{id}", s.origin(s.guard(s.deletePlan, "owner", "admin", "finance")))
	mux.HandleFunc("PATCH /plans", s.origin(s.guard(s.updatePlanAmount, "owner", "admin", "finance")))
	mux.HandleFunc("POST /subscriptions", s.origin(s.guard(s.createSubscription, "owner", "admin", "finance")))
	mux.HandleFunc("PATCH /subscriptions", s.origin(s.guard(s.updateSubscription, "owner", "admin", "finance")))
	mux.HandleFunc("POST /term-activations", s.origin(s.guard(s.createActivation, "owner", "admin", "finance")))
	mux.HandleFunc("POST /term-activations/verify", s.origin(s.verifyActivation))
	mux.HandleFunc("POST /term-activations/{id}/email", s.origin(s.guard(s.emailActivation, "owner", "admin", "finance")))
	mux.HandleFunc("GET /term-activations/{id}/code", s.guard(s.activationCode, "owner", "admin", "finance"))
	mux.HandleFunc("POST /invoices", s.origin(s.guard(s.createInvoice, "owner", "admin", "finance")))
	mux.HandleFunc("POST /invoices/issue", s.origin(s.guard(s.invoiceIssue, "owner", "admin", "finance")))
	mux.HandleFunc("GET /invoices/{id}/payments", s.guard(s.listInvoicePayments))
	mux.HandleFunc("POST /payments", s.origin(s.guard(s.recordPayment, "owner", "admin", "finance")))
	mux.HandleFunc("POST /tickets", s.origin(s.guard(s.createTicket, "owner", "admin", "support")))
	mux.HandleFunc("GET /tickets/assignees", s.guard(s.ticketAssignees, "owner", "admin", "support"))
	mux.HandleFunc("GET /tickets/{id}", s.guard(s.ticketDetail))
	mux.HandleFunc("PATCH /tickets", s.origin(s.guard(s.ticketUpdate, "owner", "admin", "support")))
	mux.HandleFunc("POST /tickets/replies", s.origin(s.guard(s.ticketReply, "owner", "admin", "support")))
	mux.HandleFunc("POST /operators", s.origin(s.guard(s.createOperator, "owner", "admin")))
	host := os.Getenv("LISTEN_HOST")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{Addr: net.JoinHostPort(host, port), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("API listening on :%s", strconv.Quote(port))
	log.Fatal(srv.ListenAndServe())
}
