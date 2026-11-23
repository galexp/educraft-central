CREATE TABLE IF NOT EXISTS operators (
 id UUID PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL UNIQUE,
 password_hash TEXT NOT NULL, role TEXT NOT NULL CHECK(role IN ('owner','admin','finance','support','viewer')),
 active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash TEXT PRIMARY KEY, operator_id UUID NOT NULL REFERENCES operators(id) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS schools (
 id UUID PRIMARY KEY, name TEXT NOT NULL, code TEXT NOT NULL UNIQUE,
 contact_name TEXT NOT NULL DEFAULT '', contact_email TEXT NOT NULL DEFAULT '',
 contact_phone TEXT NOT NULL DEFAULT '', domain TEXT NOT NULL DEFAULT '',
 hosting TEXT NOT NULL DEFAULT 'self_hosted' CHECK(hosting IN ('self_hosted','techknights')),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','active','suspended','archived')),
 notes TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS plans (
 id UUID PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '',
 amount_minor BIGINT NOT NULL CHECK(amount_minor >= 0), unit_count BIGINT NOT NULL DEFAULT 0 CHECK(unit_count >= 0), unit_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK(unit_amount_minor >= 0), currency CHAR(3) NOT NULL DEFAULT 'NGN',
 interval TEXT NOT NULL CHECK(interval = 'term'),
 active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS subscriptions (
 id UUID PRIMARY KEY, school_id UUID NOT NULL REFERENCES schools(id), plan_id UUID NOT NULL REFERENCES plans(id),
 status TEXT NOT NULL DEFAULT 'trial' CHECK(status IN ('trial','active','past_due','cancelled','expired')),
 starts_on DATE NOT NULL, ends_on DATE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS features (
 id UUID PRIMARY KEY, feature_key TEXT NOT NULL UNIQUE, name TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS plan_features (
 plan_id UUID NOT NULL REFERENCES plans(id) ON DELETE CASCADE, feature_id UUID NOT NULL REFERENCES features(id) ON DELETE CASCADE,
 PRIMARY KEY(plan_id, feature_id)
);
CREATE TABLE IF NOT EXISTS subscription_plans (
 subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE, plan_id UUID NOT NULL REFERENCES plans(id),
 PRIMARY KEY(subscription_id, plan_id)
);
CREATE TABLE IF NOT EXISTS term_activations (
 id UUID PRIMARY KEY, school_id UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
 subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
 term_name TEXT NOT NULL, starts_on DATE NOT NULL, ends_on DATE NOT NULL,
 activation_code_hash TEXT NOT NULL UNIQUE, activation_code_ciphertext TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'issued' CHECK(status IN ('issued','active','expired','revoked')),
 activated_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE term_activations ADD COLUMN IF NOT EXISTS activation_code_ciphertext TEXT NOT NULL DEFAULT '';
INSERT INTO subscription_plans(subscription_id, plan_id) SELECT id, plan_id FROM subscriptions WHERE plan_id IS NOT NULL ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS invoices (
 id UUID PRIMARY KEY, invoice_no TEXT UNIQUE NOT NULL, school_id UUID NOT NULL REFERENCES schools(id),
 subscription_id UUID REFERENCES subscriptions(id), description TEXT NOT NULL,
 amount_minor BIGINT NOT NULL CHECK(amount_minor >= 0), currency CHAR(3) NOT NULL DEFAULT 'NGN',
 status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','issued','part_paid','paid','void')),
 due_on DATE NOT NULL, issued_on DATE, activation_id UUID UNIQUE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS activation_id UUID UNIQUE;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS unit_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS unit_amount_minor BIGINT NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS payments (
 id UUID PRIMARY KEY, invoice_id UUID NOT NULL REFERENCES invoices(id), amount_minor BIGINT NOT NULL CHECK(amount_minor > 0),
 reference TEXT NOT NULL UNIQUE, student_number TEXT NOT NULL DEFAULT '', paid_on DATE NOT NULL, recorded_by UUID NOT NULL REFERENCES operators(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE payments ADD COLUMN IF NOT EXISTS student_number TEXT NOT NULL DEFAULT '';
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_reference_key;
CREATE TABLE IF NOT EXISTS school_instance_credentials (
 id UUID PRIMARY KEY, school_id UUID NOT NULL REFERENCES schools(id), instance_id TEXT NOT NULL,
 token_hash TEXT NOT NULL UNIQUE, expires_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), last_used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS school_instance_credentials_lookup ON school_instance_credentials(token_hash, expires_at);
CREATE TABLE IF NOT EXISTS tickets (
 id UUID PRIMARY KEY, ticket_no TEXT NOT NULL UNIQUE, school_id UUID NOT NULL REFERENCES schools(id),
 subject TEXT NOT NULL, description TEXT NOT NULL,
 category TEXT NOT NULL DEFAULT 'general', priority TEXT NOT NULL DEFAULT 'normal' CHECK(priority IN ('low','normal','high','urgent')),
 status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','assigned','in_progress','awaiting_customer','resolved','closed')),
 assigned_to UUID REFERENCES operators(id), requester_name TEXT NOT NULL DEFAULT '', requester_email TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS ticket_replies (
 id UUID PRIMARY KEY, ticket_id UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
 author_id UUID NOT NULL REFERENCES operators(id), body TEXT NOT NULL, internal BOOLEAN NOT NULL DEFAULT FALSE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS school_ticket_replies (
 id UUID PRIMARY KEY, ticket_id UUID NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
 instance_id TEXT NOT NULL, body TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS audit_events (
 id UUID PRIMARY KEY, actor_id UUID REFERENCES operators(id), action TEXT NOT NULL,
 entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, details JSONB NOT NULL DEFAULT '{}'::jsonb,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS installation_invitations (
 id UUID PRIMARY KEY, school_id UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
 token_hash TEXT NOT NULL UNIQUE, token_ciphertext TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'approved' CHECK(status IN ('approved','consumed','revoked')),
 expires_at TIMESTAMPTZ NOT NULL,
 instance_id TEXT NOT NULL DEFAULT '', instance_url TEXT NOT NULL DEFAULT '',
 created_by UUID NOT NULL REFERENCES operators(id), consumed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE installation_invitations ADD COLUMN IF NOT EXISTS token_ciphertext TEXT NOT NULL DEFAULT '';
UPDATE plans SET interval='term' WHERE interval <> 'term';
ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_interval_check;
ALTER TABLE plans ADD CONSTRAINT plans_interval_check CHECK(interval = 'term');
INSERT INTO features(id,feature_key,name) VALUES
 ('10000000-0000-4000-8000-000000000001','finance_fees','Finance & Fees'),
 ('10000000-0000-4000-8000-000000000002','payroll','Payroll'),
 ('10000000-0000-4000-8000-000000000003','cbt_exams','CBT Exams & Results'),
 ('10000000-0000-4000-8000-000000000004','reports','Reports'),
 ('10000000-0000-4000-8000-000000000005','library_inventory','Library & Inventory')
 ON CONFLICT(feature_key) DO NOTHING;
INSERT INTO plans(id,name,description,amount_minor,currency,interval) VALUES
 ('20000000-0000-4000-8000-000000000001','Finance & Fees','Fee management and finance operations',0,'NGN','term'),
 ('20000000-0000-4000-8000-000000000002','Payroll','Payroll and salary management',0,'NGN','term'),
 ('20000000-0000-4000-8000-000000000003','CBT Exams & Results','Computer-based testing and results',0,'NGN','term'),
 ('20000000-0000-4000-8000-000000000004','Reports','Academic and examination reports',0,'NGN','term'),
 ('20000000-0000-4000-8000-000000000005','Library & Inventory','Library and stock management',0,'NGN','term')
 ON CONFLICT(name) DO NOTHING;
INSERT INTO plan_features(plan_id,feature_id) SELECT p.id,f.id FROM plans p JOIN features f ON f.feature_key=CASE p.name WHEN 'Finance & Fees' THEN 'finance_fees' WHEN 'Payroll' THEN 'payroll' WHEN 'CBT Exams & Results' THEN 'cbt_exams' WHEN 'Reports' THEN 'reports' WHEN 'Library & Inventory' THEN 'library_inventory' END ON CONFLICT DO NOTHING;
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_tickets_school ON tickets(school_id);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
CREATE INDEX IF NOT EXISTS idx_invoices_school ON invoices(school_id);
CREATE INDEX IF NOT EXISTS idx_subs_school ON subscriptions(school_id);
CREATE INDEX IF NOT EXISTS idx_installation_invitations_school ON installation_invitations(school_id);
CREATE INDEX IF NOT EXISTS idx_installation_invitations_status ON installation_invitations(status);
