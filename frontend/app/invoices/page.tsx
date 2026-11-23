'use client';

import { FormEvent, useEffect, useState } from 'react';
import { useOperatorRole } from '@/components/Shell';
import { Crud } from '@/components/Crud';
import { api, money, post, Row, str } from '@/lib/api';
import layout from './invoices.module.css';
import styles from '@/components/Records.module.css';
import shell from '@/components/Shell.module.css';

function InvoiceForm({ schools, onSaved }: { schools: Row[]; onSaved: () => void }) {
  const [open, setOpen] = useState(false), [schoolID, setSchoolID] = useState(''), [description, setDescription] = useState(''), [units, setUnits] = useState(''), [unitAmount, setUnitAmount] = useState(''), [currency, setCurrency] = useState('NGN'), [dueOn, setDueOn] = useState(new Date().toISOString().slice(0, 10)), [paid, setPaid] = useState(''), [reference, setReference] = useState(''), [error, setError] = useState(''), [saving, setSaving] = useState(false);
  const total = Number(units) * Math.round(Number(unitAmount) * 100);
  const paidMinor = Math.round(Number(paid) * 100);
  async function save(event: FormEvent) { event.preventDefault(); setSaving(true); setError(''); try { await post('/invoices', { school_id: schoolID, description, unit_count: Number(units), unit_amount_minor: Math.round(Number(unitAmount) * 100), currency, due_on: dueOn, amount_paid_minor: Math.round(Number(paid || 0) * 100), payment_reference: reference }); setOpen(false); onSaved(); } catch (err) { setError((err as Error).message); } finally { setSaving(false); } }
  return <>
<button className="btn" type="button" onClick={() => setOpen(true)}>+ Create invoice</button>{open && <div className={shell.modalBackdrop} role="presentation" onMouseDown={() => setOpen(false)}>
<section className={`${shell.modal} ${layout.dialog}`} role="dialog" aria-modal="true" aria-labelledby="invoice-title" onMouseDown={event => event.stopPropagation()}>
<button className={shell.modalClose} type="button" aria-label="Close" onClick={() => setOpen(false)}>×</button>
<h2 id="invoice-title">Create invoice</h2>
<p>Set the student count and amount per student. An initial payment can be recorded here.</p>
<form onSubmit={save}>
<div className={`form ${layout.fields}`}>
<label className="field">School<select required value={schoolID} onChange={event => setSchoolID(event.target.value)}>
<option value="">Select school…</option>{schools.map(school => <option key={str(school.id)} value={str(school.id)}>{str(school.name)}</option>)}</select>
</label>
<label className="field wide">Description<input required value={description} onChange={event => setDescription(event.target.value)} />
</label>
<label className="field">Number of students<input required min="1" type="number" value={units} onChange={event => setUnits(event.target.value)} />
</label>
<label className="field">Amount per student<input required min="0" step="0.01" type="number" value={unitAmount} onChange={event => setUnitAmount(event.target.value)} />
</label>
<label className="field">Currency<select value={currency} onChange={event => setCurrency(event.target.value)}>
<option>NGN</option>
<option>GBP</option>
<option>USD</option>
</select>
</label>
<label className="field">Due date<input required type="date" value={dueOn} onChange={event => setDueOn(event.target.value)} />
</label>
<label className="field">Initial amount paid<input min="0" step="0.01" type="number" value={paid} onChange={event => setPaid(event.target.value)} />
</label>{Number(paid) > 0 && <label className="field">Payment reference <span className="muted">(optional)</span><input value={reference} onChange={event => setReference(event.target.value)} />
</label>}</div>
<dl className={layout.summary}>
<div>
<dt>Total</dt>
<dd>{money(total, currency)}</dd>
</div>
<div>
<dt>Paid</dt>
<dd>{money(paidMinor, currency)}</dd>
</div>
<div>
<dt>Balance due</dt>
<dd>{money(Math.max(0, total - paidMinor), currency)}</dd>
</div>
</dl>{error && <div className="error">{error}</div>}<div className={shell.modalActions}>
<button className={shell.modalCancel} type="button" onClick={() => setOpen(false)}>Cancel</button>
<button className={shell.modalConfirm} disabled={saving}>{saving ? 'Creating…' : 'Create invoice'}</button>
</div>
</form>
</section>
</div>}</>;
}

function PaymentForm({ row, onSaved }: { row: Row; onSaved: () => void }) {
  const [open, setOpen] = useState(false), [amount, setAmount] = useState(''), [studentNumber, setStudentNumber] = useState(str(row.unit_count || '')), [reference, setReference] = useState(''), [error, setError] = useState(''), [saving, setSaving] = useState(false);
  const unitsSet = Number(row.unit_count || 0) > 0;
  const perUnitMinor = Number(row.unit_amount_minor || 0) || (Number(row.unit_count || 0) > 0 ? Number(row.amount_minor || 0) / Number(row.unit_count) : Number(row.amount_minor || 0));
  const expectedMinor = Math.max(0, Number(studentNumber || 0) * perUnitMinor);
  async function save(event: FormEvent) {
    event.preventDefault(); setSaving(true); setError('');
    try {
      await post('/payments', { invoice_id: row.id, amount_minor: Math.round(Number(amount) * 100), unit_count: Number(studentNumber), student_number: studentNumber, reference, paid_on: new Date().toISOString().slice(0, 10) });
      setOpen(false); setAmount(''); setStudentNumber(str(row.unit_count || '')); setReference(''); onSaved();
    } catch (err) { setError((err as Error).message); } finally { setSaving(false); }
  }
  return <>
    <button className="btn secondary" type="button" onClick={() => setOpen(true)}>Record payment</button>
    {open && <div className={shell.modalBackdrop} role="presentation" onMouseDown={() => setOpen(false)}>
      <section className={`${shell.modal} ${layout.paymentDialog}`} role="dialog" aria-modal="true" aria-labelledby={`payment-title-${str(row.id)}`} onMouseDown={event => event.stopPropagation()}>
        <button className={shell.modalClose} type="button" aria-label="Close" onClick={() => setOpen(false)}>×</button>
        <h2 id={`payment-title-${str(row.id)}`}>Record payment</h2>
        <p>Enter the amount received for invoice {str(row.invoice_no)} and a payment reference.</p>
        <form onSubmit={save}>
        <div className={`form ${layout.paymentFields}`}>
          <label className="field wide">Number of students / units<input required={!unitsSet} readOnly={unitsSet} min="1" step="1" type="number" value={studentNumber} onChange={event => setStudentNumber(event.target.value)} /></label>
          <div className={layout.expectedPayment}><span>Expected payment<br /><small>{money(perUnitMinor, str(row.currency || 'NGN'))} per student/unit</small></span><strong>{money(expectedMinor, str(row.currency || 'NGN'))}</strong></div>
          <div className={layout.expectedPayment}><span>Balance before payment</span><strong>{money(Math.max(0, expectedMinor - Number(row.paid_minor || 0)), str(row.currency || 'NGN'))}</strong></div>
          <label className="field wide">Amount paid<input required min="0.01" step="0.01" type="number" value={amount} onChange={event => setAmount(event.target.value)} /></label>
          <label className="field wide">Payment reference<input required value={reference} onChange={event => setReference(event.target.value)} placeholder="e.g. bank transfer reference" /></label>
        </div>
          {error && <div className="error">{error}</div>}
          <div className={shell.modalActions} style={{ marginTop: 38 }}><button className={shell.modalCancel} type="button" onClick={() => setOpen(false)}>Cancel</button><button className={shell.modalConfirm} disabled={saving}>{saving ? 'Saving…' : 'Save payment'}</button></div>
        </form>
      </section>
    </div>}
  </>;
}

function InvoiceDetails({ row }: { row: Row }) {
  const [open, setOpen] = useState(false), [payments, setPayments] = useState<Row[]>([]), [paymentsError, setPaymentsError] = useState('');
  const currency = str(row.currency || 'NGN');
  const total = Number(row.amount_minor || 0), paid = Number(row.paid_minor || 0);
  useEffect(() => { if (!open) return; setPaymentsError(''); api<Row[]>(`/invoices/${str(row.id)}/payments`).then(setPayments).catch(err => setPaymentsError((err as Error).message)); }, [open, row.id]);
  return <>
    <button className="btn secondary" type="button" onClick={() => setOpen(true)}>View invoice</button>
    {open && <div className={shell.modalBackdrop} role="presentation" onMouseDown={() => setOpen(false)}>
      <section className={`${shell.modal} ${layout.invoiceDetails}`} role="dialog" aria-modal="true" aria-labelledby={`invoice-details-${str(row.id)}`} onMouseDown={event => event.stopPropagation()}>
        <button className={shell.modalClose} type="button" aria-label="Close" onClick={() => setOpen(false)}>×</button>
        <h2 id={`invoice-details-${str(row.id)}`}>Invoice details</h2>
        <p>{str(row.invoice_no)} · {str(row.school_name)}</p>
        <dl className={layout.invoiceGrid}>
          <div><dt>Invoice number</dt><dd>{str(row.invoice_no)}</dd></div>
          <div><dt>School</dt><dd>{str(row.school_name)}</dd></div>
          <div><dt>Description</dt><dd>{str(row.description)}</dd></div>
          <div><dt>Students / units</dt><dd>{str(row.unit_count || 0)}</dd></div>
          <div><dt>Invoice total</dt><dd>{money(total, currency)}</dd></div>
          <div><dt>Paid</dt><dd>{money(paid, currency)}</dd></div>
          <div><dt>Balance due</dt><dd>{money(Math.max(0, total - paid), currency)}</dd></div>
          <div><dt>Status</dt><dd>{str(row.status).replaceAll('_', ' ')}</dd></div>
          <div><dt>Due date</dt><dd>{str(row.due_on).slice(0, 10)}</dd></div>
        </dl>
        <h3 className={layout.paymentsTitle}>Payment transactions</h3>
        {paymentsError && <div className="error">{paymentsError}</div>}
        {payments.length === 0 && !paymentsError ? <div className="muted">No payments recorded yet.</div> : <div className={layout.transactions}><table><thead><tr><th>Date</th><th>Amount</th><th>Student/unit</th><th>Reference</th></tr></thead><tbody>{payments.map(payment => <tr key={str(payment.id)}><td>{str(payment.paid_on).slice(0, 10)}</td><td>{money(payment.amount_minor, currency)}</td><td>{str(payment.student_number) || '—'}</td><td>{str(payment.reference) || '—'}</td></tr>)}</tbody></table></div>}
        <div className={shell.modalActions}><button className={shell.modalCancel} type="button" onClick={() => setOpen(false)}>Close</button><button className={shell.modalConfirm} type="button" onClick={() => window.print()}>Print invoice</button></div>
      </section>
    </div>}
  </>;
}

export default function Invoices() {
  const canManage = ['owner', 'admin', 'finance'].includes(useOperatorRole());
  const [schools, setSchools] = useState<Row[]>([]), [reload, setReload] = useState(0);
  useEffect(() => { api<Row[]>('/schools').then(setSchools).catch(() => {}); }, []);
  return <section className={styles.records}><div className="bar"><div><h1 className="top-title">Invoices</h1><div className="subtitle">Manage student invoices and payments</div></div>{canManage && <InvoiceForm schools={schools} onSaved={() => setReload(value => value + 1)} />}</div>
    <Crud key={reload} title="Invoices" subtitle="Create invoices by student units and track partial payments" path="/invoices" canCreate={false} hideHeader fields={[]} cols={[{ key: 'invoice_no', label: 'Invoice #' }, { key: 'school_name', label: 'School' }, { key: 'description', label: 'Description' }, { key: 'unit_count', label: 'Students' }, { key: 'amount_minor', label: 'Amount', format: 'money' }, { key: 'paid_minor', label: 'Paid', format: 'money' }, { key: 'status', label: 'Status', format: 'status' }, { key: 'due_on', label: 'Due', format: 'date' }]} afterCreate={canManage ? (row, reloadRows) => <><InvoiceDetails row={row} />{row.status === 'draft' && <button className="btn secondary" type="button" onClick={async () => { try { await post('/invoices/issue', { id: row.id }); reloadRows(); } catch (err) { alert((err as Error).message); } }}>Issue</button>}{['issued', 'part_paid'].includes(str(row.status)) && <PaymentForm row={row} onSaved={reloadRows} />}</> : undefined} />
  </section>;
}
