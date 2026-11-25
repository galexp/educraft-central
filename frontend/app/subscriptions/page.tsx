'use client';
import {useEffect, useState, FormEvent} from 'react';
import {useOperatorRole} from '@/components/Shell';
import {Crud} from '@/components/Crud';
import {api, patch, post, Row, money, str} from '@/lib/api';
import records from '@/components/Records.module.css';
import styles from './subscriptions.module.css';
import shell from '@/components/Shell.module.css';

const modules: Record<string, string[]> = {
  'Finance & Fees': ['Fee setup and allocations', 'Instalments and concessions', 'Payments, receipts and balances', 'Accounts, income and expenses', 'Finance reports'],
  Payroll: ['Pay heads and payroll templates', 'Salary structures', 'Payroll generation', 'Transactions and history'],
  'CBT Exams & Results': ['Question banks', 'Timed exams and attempts', 'Exam monitoring', 'Grading and published results'],
  Reports: ['Academic report cards', 'Report templates', 'Exam analysis', 'Examination PDFs'],
  'Library & Inventory': ['Library catalogue and barcodes', 'Loans, returns and late-fee settings', 'Stock and purchases', 'Transfers and vendors'],
};

function ModuleCard({ item, canManage }: { item: Row; canManage: boolean }) {
  const name = str(item.name);
  const currency = str(item.currency);
  const [savedAmount, setSavedAmount] = useState(Number(item.amount_minor));
  const [amount, setAmount] = useState(String(savedAmount / 100));
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  async function save(event: FormEvent) {
    event.preventDefault();
    const minor = Math.round(Number(amount) * 100);
    setSaving(true);
    setError('');
    try {
      await patch('/plans', { id: item.id, amount_minor: minor });
      setSavedAmount(minor);
      setMessage('Amount saved.');
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  }

  return <li className={styles.module}>
    <h3>{name}</h3>
    <p className={styles.price}><strong>{money(savedAmount, currency)}</strong> <span>per term</span></p>
    {modules[name] ? <ul className={styles.features}>{modules[name].map(feature => <li key={feature}>{feature}</li>)}</ul> : <p>{str(item.description)}</p>}
    {canManage && <form onSubmit={save} className={styles.planForm}>
      <label className="field">Amount ({currency})
        <input type="number" required min="0" step="0.01" value={amount} disabled={saving} onChange={event => { setAmount(event.target.value); setMessage(''); }} />
      </label>
      <button className="btn secondary" disabled={saving || Math.round(Number(amount) * 100) === savedAmount}>{saving ? 'Saving…' : 'Save amount'}</button>
      {message && <small role="status">{message}</small>}
      {error && <small role="alert">{error}</small>}
    </form>}
  </li>;
}

function ActivationAction({ row }: { row: Row }) {
  const [busy, setBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [termName, setTermName] = useState('2026/2027 First Term');
  const [startsOn, setStartsOn] = useState(str(row.starts_on).slice(0, 10));
  const [endsOn, setEndsOn] = useState(str(row.ends_on).slice(0, 10));
  const [code, setCode] = useState('');
  const [activationID, setActivationID] = useState(str(row.activation_id));
  const [invoiceNo, setInvoiceNo] = useState('');
  const [error, setError] = useState('');
  async function generate() {
    setBusy(true);
    setError('');
    try {
      const result = await post('/term-activations', { subscription_id: row.id, term_name: termName, starts_on: startsOn, ends_on: endsOn });
      setCode(str(result.activation_code));
      setActivationID(str(result.id));
      setInvoiceNo(str(result.invoice_no));
      setOpen(true);
    } catch (err) { setError((err as Error).message); } finally { setBusy(false); }
  }
  async function emailCode() {
    setBusy(true); setError('');
    try { await api(`/term-activations/${encodeURIComponent(activationID)}/email`, { method: 'POST', body: JSON.stringify({ activation_code: code }) }); setError('Email sent.'); }
    catch (err) { setError((err as Error).message); } finally { setBusy(false); }
  }
  async function openActivation() {
    setOpen(true); setError(''); setCode('');
    if (!activationID) return;
    try { const result = await api<{ activation_code: string }>(`/term-activations/${encodeURIComponent(activationID)}/code`); setCode(result.activation_code); }
    catch (err) { setError((err as Error).message); }
  }
  if (str(row.activation_status) === 'active') return <span className="badge active">Term activated</span>;
  return <>
    <button className="btn secondary" type="button" onClick={openActivation} disabled={busy}>Activation code</button>
    {open && <div className={shell.modalBackdrop} role="presentation" onMouseDown={() => setOpen(false)}><section className={`${shell.modal} ${shell.invitationModal}`} role="dialog" aria-modal="true" aria-labelledby="activation-title" onMouseDown={event => event.stopPropagation()}>
      <button className={shell.modalClose} type="button" aria-label="Close" onClick={() => setOpen(false)}>×</button>
      <h2 id="activation-title">Term activation</h2>
      {!code ? <><p>Issue an activation code for this subscription and academic term. An issued invoice is created automatically.</p><div className="form"><label className="field">Term or season<input value={termName} onChange={event => setTermName(event.target.value)} /></label><label className="field">Start date<input type="date" value={startsOn} onChange={event => setStartsOn(event.target.value)} /></label><label className="field">Expiry date<input type="date" value={endsOn} onChange={event => setEndsOn(event.target.value)} /></label></div>{error && <div className="error tiny">{error}</div>}<div className={shell.modalActions}><button className={shell.modalCancel} type="button" onClick={() => setOpen(false)}>Cancel</button><button className={shell.modalConfirm} type="button" onClick={generate} disabled={busy || !termName || !startsOn || !endsOn}>{busy ? 'Generating…' : 'Generate code'}</button></div></> : <><p>Copy or send this code to the school administrator.</p><div className="success tiny">Invoice {invoiceNo} created and issued.</div><label className="field">Activation code<input value={code} readOnly onFocus={event => event.currentTarget.select()} /></label>{error && <div className="success tiny">{error}</div>}<div className={shell.modalActions}><button className={shell.modalCancel} type="button" onClick={() => setOpen(false)}>Close</button><button className={shell.modalCancel} type="button" onClick={emailCode} disabled={busy}>{busy ? 'Sending…' : 'Send email'}</button><button className={shell.modalConfirm} type="button" onClick={() => navigator.clipboard.writeText(code)}>Copy code</button></div></>}
    </section></div>}
  </>;
}

export default function Subscriptions() {
  const canManage = ['owner', 'admin', 'finance'].includes(useOperatorRole());
  const [schools, setSchools] = useState<Row[]>([]);
  const [plans, setPlans] = useState<Row[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    api<Row[]>('/schools').then(setSchools).catch(err => setError(err.message));
    api<Row[]>('/plans').then(setPlans).catch(err => setError(err.message)).finally(() => setLoading(false));
  }, []);
  return (
    <div className={records.records}>
      <Crud canCreate={canManage} editable={canManage} title="Subscriptions" subtitle="Manage school subscriptions and billing plans" path="/subscriptions" cols={[{key:'school_name',label:'School'},{key:'plan_name',label:'Plans'},{key:'status',label:'Status',format:'status'},{key:'starts_on',label:'Starts',format:'date'},{key:'ends_on',label:'Ends',format:'date'}]} fields={[{name:'school_id',label:'School',type:'select',required:true,options:schools.map(x=>({value:str(x.id),label:str(x.name)}))},{name:'plan_ids',label:'Plans (select one or more)',type:'select',multiple:true,required:true,options:plans.map(x=>({value:str(x.id),label:str(x.name)}))},{name:'status',label:'Status',type:'select',defaultValue:'trial',options:['trial','active','past_due','cancelled','expired'].map(x=>({value:x,label:x}))},{name:'starts_on',label:'Start date',type:'date',required:true},{name:'ends_on',label:'End date',type:'date'}]} afterCreate={canManage ? row => <ActivationAction row={row} /> : undefined}/>
      <section className={`panel ${styles.plans}`} aria-labelledby="plans-heading">
        <h2 id="plans-heading">School modules</h2>
        {error && <div className="error" role="alert">{error}</div>}
        {plans.length > 0 ? (
          <ul className={styles.planList}>
            {plans.map(item => <ModuleCard key={str(item.id)} item={item} canManage={canManage} />)}
          </ul>
        ) : !error && <p className="muted">{loading ? 'Loading modules…' : 'No modules available.'}</p>}
      </section>
    </div>
  );
}
