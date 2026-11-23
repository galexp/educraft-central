'use client';

import { useState, type FormEvent } from 'react';
import Image from 'next/image';
import { useRouter } from 'next/navigation';
import { ArrowRight, Eye, EyeOff, LockKeyhole, Mail } from 'lucide-react';
import { api } from '@/lib/api';
import logo from '../../logo-web.png';
import styles from './login.module.css';

export default function Login() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const router = useRouter();

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError('');

    try {
      await api('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ email, password }),
      });
      router.replace('/');
      router.refresh();
    } catch (error) {
      setError((error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className={styles.login}>
      <section className={styles.signIn} aria-labelledby="login-heading">
        <div className={styles.content}>
          <Image className={styles.logo} src={logo} alt="EduCraft360" priority unoptimized />
          <h1 id="login-heading">Welcome back</h1>
          <p className={styles.intro}>Sign in to the central management portal.</p>

          <form className={styles.form} onSubmit={submit}>
            {error && <p className={styles.error} role="alert">{error}</p>}
            <div className={styles.field}>
              <label htmlFor="login-email">Email address</label>
              <div className={styles.input}>
                <Mail size={18} aria-hidden="true" />
                <input id="login-email" name="email" type="email" placeholder="you@example.com"
                  required value={email} onChange={event => setEmail(event.target.value)} autoComplete="username" />
              </div>
            </div>

            <div className={styles.field}>
              <label htmlFor="login-password">Password</label>
              <div className={styles.input}>
                <LockKeyhole size={18} aria-hidden="true" />
                <input id="login-password" name="password" type={showPassword ? 'text' : 'password'}
                  required value={password} onChange={event => setPassword(event.target.value)} autoComplete="current-password" />
                <button className={styles.visibility} type="button" aria-controls="login-password"
                  aria-label={showPassword ? 'Hide password' : 'Show password'}
                  onClick={() => setShowPassword(value => !value)}>
                  {showPassword ? <EyeOff size={19} aria-hidden="true" /> : <Eye size={19} aria-hidden="true" />}
                </button>
              </div>
            </div>

            <button className={styles.submit} type="submit" disabled={busy}>
              {busy ? 'Signing in…' : 'Sign in'}
              <ArrowRight size={18} aria-hidden="true" />
            </button>
          </form>

          <p className={styles.help}>Need an account? Contact your platform administrator.</p>
        </div>
      </section>

      <aside className={styles.brandPanel} aria-label="About EduCraft360 Central">
        <div className={styles.art} aria-hidden="true">
          <div className={styles.orbit} />
          <div className={styles.orbit} />
          <div className={styles.orbit} />
          <div className={styles.emblem}>
            <Image src="/brand-icon.png" alt="" width={72} height={72} unoptimized />
          </div>
        </div>
        <div className={styles.brandCopy}>
          <p>EduCraft360 Central</p>
          <h2>School operations,<br />in one place.</h2>
          <div>Manage school accounts, billing, and support from one workspace.</div>
        </div>
      </aside>
    </main>
  );
}
