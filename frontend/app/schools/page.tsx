'use client';

import { useState } from 'react';
import { useOperatorRole } from '@/components/Shell';
import { Crud } from '@/components/Crud';
import { patch, post, str, Row } from '@/lib/api';
import records from '@/components/Records.module.css';
import shell from '@/components/Shell.module.css';

function InvitationAction({ row }: { row: Row }) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [url, setUrl] = useState('');
  const [expiresInHours, setExpiresInHours] = useState('24');
  const [copied, setCopied] = useState(false);
  if (str(row.status) === 'active') return null;

  async function createInvitation() {
    setLoading(true);
    setError('');
    try {
      const result = await post('/installation-invitations', { school_id: str(row.id), expires_in_hours: Number(expiresInHours) });
      setUrl(str(result.install_url));
      setOpen(true);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  async function copyLink() {
    if (!url) return;
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(url);
      } else {
        const input = document.createElement('textarea');
        input.value = url;
        input.style.position = 'fixed';
        input.style.opacity = '0';
        document.body.appendChild(input);
        input.select();
        document.execCommand('copy');
        input.remove();
      }
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      setError('Unable to copy automatically. Select the link and copy it manually.');
    }
  }

  function openModal() {
    setError('');
    setUrl('');
    setCopied(false);
    setOpen(true);
  }

  return <>
    <button className="btn secondary" type="button" onClick={openModal}>Invite</button>
    {open && <div className={shell.modalBackdrop} role="presentation" onMouseDown={() => setOpen(false)}>
      <section className={`${shell.modal} ${shell.invitationModal}`} role="dialog" aria-modal="true" aria-labelledby="invitation-title" onMouseDown={event => event.stopPropagation()}>
        <button className={shell.modalClose} type="button" aria-label="Close" onClick={() => setOpen(false)}>×</button>
        <h2 id="invitation-title">Installation invitation</h2>
        <p>Generate a one-time installation link for this school administrator.</p>
        {!url && <label className="field">Link expiry<select value={expiresInHours} onChange={event => setExpiresInHours(event.target.value)}><option value="24">24 hours</option><option value="72">3 days</option><option value="168">7 days</option><option value="720">30 days</option></select></label>}
        {url && <label className="field">Install URL<input value={url} readOnly onFocus={event => event.currentTarget.select()} /> </label>}
        {error && <div className="error tiny">{error}</div>}
        <div className={shell.modalActions}><button className={shell.modalCancel} type="button" onClick={() => setOpen(false)}>Close</button>{url ? <button className={shell.modalConfirm} type="button" onClick={copyLink}>{copied ? 'Copied' : 'Copy link'}</button> : <button className={shell.modalConfirm} type="button" onClick={createInvitation} disabled={loading}>{loading ? 'Generating…' : 'Generate invitation'}</button>}</div>
      </section>
    </div>}
  </>;
}

export default function Schools() {
  const canManage = ['owner', 'admin'].includes(useOperatorRole());
  return <section className={records.records}><Crud canCreate={canManage} title="Schools" subtitle="Manage school accounts, hosting, status, and installation access" path="/schools" cols={[{ key: 'code', label: 'Generated code' }, { key: 'name', label: 'School' }, { key: 'contact_email', label: 'Contact email' }, { key: 'domain', label: 'Domain' }, { key: 'hosting', label: 'Hosting' }, { key: 'status', label: 'Status', format: 'status' }]} fields={[{ name: 'name', label: 'School name', required: true }, { name: 'contact_name', label: 'Contact name' }, { name: 'contact_email', label: 'Contact email', type: 'email' }, { name: 'contact_phone', label: 'Contact phone' }, { name: 'domain', label: 'School domain', required: true }, { name: 'hosting', label: 'Hosting', type: 'select', required: true, defaultValue: 'self_hosted', options: [{ value: 'self_hosted', label: 'Self-hosted' }, { value: 'techknights', label: 'TechKnights-hosted' }] }, { name: 'notes', label: 'Notes', type: 'textarea' }]} afterCreate={canManage ? (row: Row, reload) => <div className="toolbar"><select title="Change school status" value="" onChange={async event => { const nextStatus = event.target.value; if (!nextStatus) return; if (!window.confirm(`Change this school status to ${nextStatus}?`)) return; try { await patch('/schools/status', { id: row.id, status: nextStatus }); reload(); } catch (err) { alert((err as Error).message); } }}>{['suspended', 'archived'].map(status => <option key={status} value={status}>{status}</option>)}<option value="">Change status…</option></select><InvitationAction row={row} /></div> : undefined} /></section>;
}
