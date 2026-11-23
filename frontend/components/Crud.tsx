'use client';

import { useEffect, useState, FormEvent } from 'react';
import { api, post, patch, Row, str, money } from '@/lib/api';

export type Field = { name: string; label: string; type?: 'text' | 'email' | 'date' | 'number' | 'textarea' | 'password' | 'select'; required?: boolean; multiple?: boolean; options?: { value: string; label: string }[]; defaultValue?: string };
export type Col = { key: string; label: string; format?: 'money' | 'status' | 'date' | 'datetime' };

type CrudProps = { title: string; subtitle: string; path: string; cols: Col[]; fields: Field[]; canCreate?: boolean; editable?: boolean; hideHeader?: boolean; afterCreate?: (row: Row, reload: () => void) => React.ReactNode };

export function Crud({ title, subtitle, path, cols, fields, canCreate = true, editable = false, hideHeader = false, afterCreate }: CrudProps) {
  const [rows, setRows] = useState<Row[]>([]);
  const [show, setShow] = useState(false);
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [search, setSearch] = useState('');
  const [values, setValues] = useState<Row>({});
  const reload = () => api<Row[]>(path).then(setRows).catch(e => setError(e.message));
  useEffect(() => { reload(); }, [path]);

  function beginCreate() { setValues({}); setEditing(false); setMessage(''); setError(''); setShow(true); }
  function beginEdit(row: Row) { setValues({ ...row }); setEditing(true); setMessage(''); setError(''); setShow(true); }

  async function save(event: FormEvent) {
    event.preventDefault(); setSaving(true); setError('');
    try {
      const payload = { ...values };
      for (const field of fields) {
        if (field.type === 'number') payload[field.name] = Math.round(Number(payload[field.name] || 0) * 100);
        else if (payload[field.name] === undefined && field.defaultValue !== undefined) payload[field.name] = field.defaultValue;
      }
      if (editing) await patch(path, payload); else await post(path, payload);
      setMessage(editing ? 'Updated successfully' : 'Saved successfully'); setShow(false); setValues({}); reload();
    } catch (err) { setError((err as Error).message); } finally { setSaving(false); }
  }

  const filtered = rows.filter(row => JSON.stringify(row).toLowerCase().includes(search.toLowerCase()));
  return <>
    {!hideHeader && <div className="bar"><div><h1 className="top-title">{title}</h1><div className="subtitle">{subtitle}</div></div>{canCreate && <button className="btn" onClick={show ? () => setShow(false) : beginCreate}>{show ? 'Cancel' : '+ Add new'}</button>}</div>}
    {error && <div className="error">{error}</div>}{message && <div className="success">{message}</div>}
    {show && (canCreate || editing) && <form className="panel" onSubmit={save}><h3>{editing ? `Edit ${title.replace(/s$/, '')}` : `New ${title.replace(/s$/, '')}`}</h3><div className="form">
      {fields.map(field => <label className={`field ${field.type === 'textarea' ? 'wide' : ''}`} key={field.name}>{field.label}
        {field.type === 'select' ? <select required={field.required} multiple={field.multiple} value={field.multiple ? (Array.isArray(values[field.name]) ? values[field.name] as string[] : []) : str(values[field.name] ?? field.defaultValue ?? '')} onChange={event => setValues({ ...values, [field.name]: field.multiple ? Array.from(event.target.selectedOptions, option => option.value) : event.target.value })}>{!field.multiple && <option value="">Select…</option>}{field.options?.map(option => <option value={option.value} key={option.value}>{option.label}</option>)}</select> : field.type === 'textarea' ? <textarea required={field.required} rows={4} value={str(values[field.name])} onChange={event => setValues({ ...values, [field.name]: event.target.value })} /> : <input type={field.type === 'number' ? 'number' : field.type || 'text'} min={field.type === 'number' ? '0' : undefined} step={field.type === 'number' ? '0.01' : undefined} required={field.required} value={str(values[field.name])} onChange={event => setValues({ ...values, [field.name]: event.target.value })} />}
      </label>)}
    </div><div style={{ marginTop: 18 }}><button className="btn" disabled={saving}>{saving ? 'Saving…' : editing ? 'Update record' : 'Save record'}</button></div></form>}
    <div className="card"><div className="bar"><strong>{filtered.length} records</strong><input placeholder="Search records…" value={search} onChange={event => setSearch(event.target.value)} style={{ padding: 10, border: '1px solid #d6e1ec', borderRadius: 8 }} /></div><div className="table-wrap"><table className="data-table"><thead><tr>{cols.map(column => <th key={column.key}>{column.label}</th>)}{(editable || afterCreate) && <th>Actions</th>}</tr></thead><tbody>{filtered.length === 0 ? <tr><td className="empty" colSpan={cols.length + 1}>No records found.</td></tr> : filtered.map((row, index) => <tr key={str(row.id ?? index)}>{cols.map(column => <td key={column.key}>{column.format === 'status' ? <span className={`badge ${str(row[column.key])}`}>{str(row[column.key]).replaceAll('_', ' ')}</span> : column.format === 'money' ? money(row[column.key], str(row.currency || 'NGN')) : column.format === 'date' ? str(row[column.key]).slice(0, 10) : column.format === 'datetime' ? (str(row[column.key]) ? new Date(str(row[column.key])).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' }) : '—') : str(row[column.key])}</td>)}{(editable || afterCreate) && <td><div className="toolbar">{editable && <button className="btn secondary" type="button" onClick={() => beginEdit(row)}>Edit</button>}{afterCreate?.(row, reload)}</div></td>}</tr>)}</tbody></table></div></div>
  </>;
}
