export type Row = Record<string, unknown>;
export async function api<T = Row[]>(path: string, opts: RequestInit = {}): Promise<T> {
 const response = await fetch(`/api${path}`, { credentials:'same-origin', cache:'no-store', ...opts,
  headers: { ...(opts.body ? {'Content-Type':'application/json'} : {}), ...opts.headers } });
 let result: unknown;
 try { result = await response.json(); } catch { throw new Error(`Server returned ${response.status}`); }
 if(!response.ok) throw new Error((result as {error?:string})?.error ?? `Request failed (${response.status})`);
 return result as T;
}
export const post = (path:string, body:Row) => api<Row>(path,{method:'POST',body:JSON.stringify(body)});
export const patch = (path:string, body:Row) => api<Row>(path,{method:'PATCH',body:JSON.stringify(body)});
export const money = (minor:unknown,currency='NGN')=>new Intl.NumberFormat('en-NG',{style:'currency',currency}).format(Number(minor||0)/100);
export const str = (x:unknown)=>x==null?'':String(x);
