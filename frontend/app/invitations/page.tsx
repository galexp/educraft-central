'use client';

import { useEffect, useState } from 'react';
import { Crud } from '@/components/Crud';
import { api, Row, str } from '@/lib/api';
import { useOperatorRole } from '@/components/Shell';
import shell from '@/components/Shell.module.css';
import styles from '@/components/Records.module.css';

function ViewLink({ row }: { row: Row }) {
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState('');
  const [error, setError] = useState('');
  const [copied, setCopied] = useState(false);
  const [sending, setSending] = useState(false);
  async function showLink() {
    try { setError(''); setCopied(false); const result = await api<{ install_url: string }>(`/installation-invitations/${encodeURIComponent(str(row.id))}/link`); setUrl(result.install_url); setOpen(true); }
    catch (err) { setError((err as Error).message); }
  }
  async function copyLink() {
    try {
      if (navigator.clipboard?.writeText) await navigator.clipboard.writeText(url);
      else {
        const input = document.createElement('textarea'); input.value = url; input.style.position = 'fixed'; input.style.opacity = '0';
        document.body.appendChild(input); input.select(); document.execCommand('copy'); input.remove();
      }
      setCopied(true); window.setTimeout(() => setCopied(false), 1800);
    } catch { setError('Unable to copy automatically. Select the link and copy it manually.'); }
  }
  async function emailLink() {
    setSending(true); setError('');
    try { await api(`/installation-invitations/${encodeURIComponent(str(row.id))}/email`, { method: 'POST', body: '{}' }); setError('Email sent.'); }
    catch (err) { setError((err as Error).message); } finally { setSending(false); }
  }
  return <>
    <button className="btn secondary" type="button" onClick={showLink}>View link</button>
    {error && <span className="error tiny">{error}</span>}
    {open && <div className={shell.modalBackdrop} role="presentation" onMouseDown={() => setOpen(false)}><section className={`${shell.modal} ${shell.invitationModal}`} role="dialog" aria-modal="true" aria-labelledby="view-link-title" onMouseDown={event => event.stopPropagation()}><button className={shell.modalClose} type="button" aria-label="Close" onClick={() => setOpen(false)}>×</button><h2 id="view-link-title">Installation link</h2><p>Copy or send this link to the school administrator.</p><label className="field">Install URL<input value={url} readOnly onFocus={event => event.currentTarget.select()} /></label>{error && <div className="success tiny">{error}</div>}<div className={shell.modalActions}><button className={shell.modalCancel} type="button" onClick={() => setOpen(false)}>Close</button><button className={shell.modalCancel} type="button" onClick={emailLink} disabled={sending}>{sending ? 'Sending…' : 'Send email'}</button><button className={shell.modalConfirm} type="button" onClick={copyLink}>{copied ? 'Copied' : 'Copy link'}</button></div></section></div>}
  </>;
}

export default function Invitations() {
  const canManage = ['owner', 'admin'].includes(useOperatorRole());
  const [schools, setSchools] = useState<Row[]>([]);
  const [schoolID, setSchoolID] = useState('');
  useEffect(() => { api<Row[]>('/schools').then(setSchools).catch(() => setSchools([])); }, []);
  return <section className={styles.records}>
    <div className="panel">
      <label className="field">Select school
        <select value={schoolID} onChange={event => setSchoolID(event.target.value)}>
          <option value="">Choose a school…</option>
          {schools.map(school => <option key={str(school.id)} value={str(school.id)}>{str(school.name)} · {str(school.code)}</option>)}
        </select>
      </label>
    </div>
    {!schoolID ? <div className="card empty">Select a school to view its installation links.</div> : <Crud
      canCreate={false}
      title="Installation links"
      subtitle="Track this school’s active, expired, consumed, and instance-bound invitations"
      path={`/invitations?school_id=${encodeURIComponent(schoolID)}`}
      cols={[
        { key: 'name', label: 'School' },
        { key: 'code', label: 'School code' },
        { key: 'availability', label: 'Availability', format: 'status' },
        { key: 'status', label: 'Status', format: 'status' },
        { key: 'expires_at', label: 'Expires', format: 'datetime' },
        { key: 'instance_id', label: 'Instance' },
        { key: 'consumed_at', label: 'Consumed', format: 'datetime' },
      ]}
      fields={[]}
      afterCreate={canManage ? (row: Row, reload) => <div className="toolbar"><ViewLink row={row} />{str(row.availability) === 'active' && <button className="btn secondary" type="button" onClick={async () => {
        if (!window.confirm('Revoke this unused installation link?')) return;
        try {
          await api(`/installation-invitations/${encodeURIComponent(str(row.id))}`, { method: 'DELETE', body: '{}' });
          reload();
        } catch (error) {
          window.alert((error as Error).message);
        }
      }}>Revoke</button>}</div> : undefined}
    />}
  </section>;
}
