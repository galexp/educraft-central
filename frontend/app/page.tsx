'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { ArrowDownRight, ArrowRight, ArrowUpRight, CircleCheck, GraduationCap, ReceiptText, Ticket, Wallet } from 'lucide-react';
import { api, money } from '@/lib/api';
import styles from './dashboard.module.css';

type Metrics = {
  schools: number;
  active_schools: number;
  tickets: number;
  open_tickets: number;
  invoices: number;
  outstanding_minor: number;
};

const metrics = [
  { key: 'schools', label: 'Registered schools', icon: GraduationCap, href: '/schools', color: 'purple' },
  { key: 'active_schools', label: 'Active schools', icon: CircleCheck, href: '/schools', color: 'green' },
  { key: 'open_tickets', label: 'Open tickets', icon: Ticket, href: '/tickets', color: 'amber' },
  { key: 'invoices', label: 'Invoices', icon: ReceiptText, href: '/invoices', color: 'blue' },
] as const;

const actions = [
  { title: 'Manage schools', description: 'View school accounts and statuses', href: '/schools', icon: GraduationCap },
  { title: 'Support tickets', description: 'Review requests and assignments', href: '/tickets', icon: Ticket },
  { title: 'Invoices', description: 'Review invoices and recorded payments', href: '/invoices', icon: ReceiptText },
] as const;

const formatCount = new Intl.NumberFormat('en');

export default function Home() {
  const [data, setData] = useState<Metrics>();
  const [error, setError] = useState('');

  useEffect(() => {
    const controller = new AbortController();
    api<Metrics>('/dashboard', { signal: controller.signal })
      .then(setData)
      .catch(error => { if (!controller.signal.aborted) setError(error.message); });
    return () => controller.abort();
  }, []);

  return (
    <div className={styles.dashboard}>
      <section className={styles.hero} aria-labelledby="dashboard-heading">
        <div className={styles.heroArt} aria-hidden="true" />
        <div className={styles.heroCopy}>
          <h1 id="dashboard-heading">Operations overview</h1>
          <p>Track schools, billing, and support across your workspace.</p>
        </div>
        <a className={styles.heroLink} href="#workspace-actions">Quick actions <ArrowDownRight size={17} aria-hidden="true" /></a>
      </section>

      {error && <div className="error" role="alert">{error}</div>}

      <section className={styles.metrics} aria-label="Key metrics" aria-busy={!data && !error}>
        {metrics.map(({ key, label, icon: Icon, href, color }) => (
          <Link key={key} href={href} className={styles.metric}>
            <div className={styles.metricTop}>
              <span className={`${styles.metricIcon} ${styles[color]}`}><Icon size={20} aria-hidden="true" /></span>
              <ArrowUpRight className={styles.metricArrow} size={17} aria-hidden="true" />
            </div>
            <strong>{data ? formatCount.format(data[key]) : '—'}</strong>
            <span className={styles.metricLabel}>{label}</span>
          </Link>
        ))}
      </section>

      <div className={styles.lower}>
        <section className={`${styles.panel} ${styles.balance}`} aria-labelledby="balance-heading">
          <span className={`${styles.metricIcon} ${styles.purple}`}><Wallet size={20} aria-hidden="true" /></span>
          <h2 id="balance-heading">Outstanding balance</h2>
          <strong className={styles.amount}>{data ? money(data.outstanding_minor) : '—'}</strong>
          <p>Unpaid amount on issued invoices in NGN.</p>
          <Link href="/invoices" className={styles.textLink}>View invoices <ArrowRight size={16} aria-hidden="true" /></Link>
        </section>

        <section className={`${styles.panel} ${styles.actions}`} id="workspace-actions" aria-labelledby="actions-heading">
          <h2 id="actions-heading">Quick actions</h2>
          {actions.map(({ title, description, href, icon: Icon }) => (
            <Link href={href} key={href} className={styles.action}>
              <span className={styles.actionIcon}><Icon size={18} aria-hidden="true" /></span>
              <span><strong>{title}</strong><small>{description}</small></span>
              <ArrowRight className={styles.actionArrow} size={17} aria-hidden="true" />
            </Link>
          ))}
        </section>
      </div>
    </div>
  );
}
