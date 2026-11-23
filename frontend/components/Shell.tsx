'use client';

import Image from 'next/image';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { createContext, useContext, useEffect, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, KeyRound, LayoutDashboard, LogOut, Receipt, School, Ticket, UserRound, Users, Wallet, X } from 'lucide-react';
import { api } from '@/lib/api';
import styles from './Shell.module.css';

type Operator = { name: string; email: string; role: string };

const OperatorRole = createContext('');
export const useOperatorRole = () => useContext(OperatorRole);

const navigation = [
  ['/', 'Overview', LayoutDashboard],
  ['/schools', 'Schools', School],
  ['/invitations', 'Installation Links', KeyRound],
  ['/subscriptions', 'Subscriptions', Wallet],
  ['/invoices', 'Invoices', Receipt],
  ['/tickets', 'Support Tickets', Ticket],
  ['/users', 'Team & Roles', Users],
] as const;
function canSeeNavigation(role: string, href: string) {
  if (role === 'owner' || role === 'admin') return true;
  if (role === 'finance') return ['/', '/subscriptions', '/invoices'].includes(href);
  if (role === 'support') return ['/', '/tickets'].includes(href);
  return href === '/';
}

export function Shell({ children }: { children: React.ReactNode }) {
  const path = usePathname();
  const router = useRouter();
  const [operator, setOperator] = useState<Operator>();
  const [signingOut, setSigningOut] = useState(false);
  const [confirmingSignOut, setConfirmingSignOut] = useState(false);
  const [accountMenuOpen, setAccountMenuOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [passwordMessage, setPasswordMessage] = useState('');
  const [changingPassword, setChangingPassword] = useState(false);
  const [error, setError] = useState('');
  const cancelSignOutRef = useRef<HTMLButtonElement>(null);
  const isLogin = path === '/login';

  useEffect(() => {
    if (isLogin) {
      setOperator(undefined);
      setError('');
      return;
    }

    let current = true;
    api<Operator>('/auth/me')
      .then(data => { if (current) setOperator(data); })
      .catch(() => { if (current) router.replace('/login'); });

    return () => { current = false; };
  }, [isLogin, router]);

  useEffect(() => {
    if (confirmingSignOut) cancelSignOutRef.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      setConfirmingSignOut(false);
      setProfileOpen(false);
      setPasswordOpen(false);
      setAccountMenuOpen(false);
    };
    window.addEventListener('keydown', closeOnEscape);
    return () => window.removeEventListener('keydown', closeOnEscape);
  }, [confirmingSignOut, profileOpen, passwordOpen]);

  async function signOut() {
    setSigningOut(true);
    setError('');
    try {
      await api('/auth/logout', { method: 'POST', body: '{}' });
      router.replace('/login');
    } catch (error) {
      setError((error as Error).message);
    } finally {
      setSigningOut(false);
    }
  }

  async function changePassword(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (newPassword !== confirmPassword) {
      setPasswordError('New passwords do not match.');
      return;
    }
    setChangingPassword(true);
    setPasswordMessage('');
    setError('');
    try {
      await api('/auth/password', { method: 'POST', body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }) });
      setPasswordOpen(false);
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      setPasswordError('');
      setPasswordMessage('Password updated. Please sign in again with your new password.');
      setTimeout(() => router.replace('/login'), 900);
    } catch (error) {
      setPasswordError((error as Error).message);
    } finally {
      setChangingPassword(false);
    }
  }

  if (isLogin) return <>{children}</>;
  if (!operator) return <div className={styles.loading} role="status">Loading workspace…</div>;

  const canManageTeam = ['owner', 'admin'].includes(operator.role);
  const currentPage = navigation.find(([href]) => path === href)?.[1];
  const initials = operator.name.trim().split(/\s+/).slice(0, 2).map(word => word[0]).join('').toUpperCase();

  return (
    <div className={styles.shell}>
      <aside className={styles.side}>
        <Link href="/" className={styles.brand}>
          <span className={styles.brandIcon}><Image src="/brand-icon.png" alt="" width={29} height={29} unoptimized /></span>
          <span><strong>EduCraft<span className={styles.brandAccent}>360</span></strong><small>Central workspace</small></span>
        </Link>
        <nav className={styles.nav} aria-label="Workspace">
          {navigation.filter(([href]) => canSeeNavigation(operator.role, href) && (href !== '/users' || canManageTeam)).map(([href, label, Icon]) => (
            <Link key={href} href={href} aria-current={path === href ? 'page' : undefined}>
              <Icon size={19} strokeWidth={1.8} aria-hidden="true" />
              {label}
            </Link>
          ))}
        </nav>
      </aside>

      <div className={styles.main}>
        <header className={styles.header}>
          <div className={styles.breadcrumb}>
            <span>Workspace</span>
            {currentPage && <><ChevronRight size={15} aria-hidden="true" /><strong>{currentPage}</strong></>}
          </div>
          <div className={styles.account}>
            <button className={styles.accountTrigger} type="button" aria-expanded={accountMenuOpen} aria-haspopup="menu" onClick={() => setAccountMenuOpen(open => !open)}>
              <span className={styles.avatar} aria-hidden="true">{initials}</span>
              <span className={styles.identity}><strong>{operator.name}</strong><small>{operator.role}</small></span>
              <ChevronDown size={16} aria-hidden="true" />
            </button>
            {accountMenuOpen && <div className={styles.accountMenu} role="menu">
              <div className={styles.menuHeader}><strong>{operator.name}</strong><small>{operator.email}</small></div>
              <button type="button" role="menuitem" onClick={() => { setProfileOpen(true); setAccountMenuOpen(false); }}><UserRound size={16} /> Profile</button>
              <button type="button" role="menuitem" onClick={() => { setPasswordOpen(true); setAccountMenuOpen(false); setPasswordMessage(''); setPasswordError(''); }}><KeyRound size={16} /> Change password</button>
              <div className={styles.menuDivider} />
              <button type="button" role="menuitem" onClick={() => { setConfirmingSignOut(true); setAccountMenuOpen(false); }}><LogOut size={16} /> Sign out</button>
            </div>}
          </div>
        </header>
        <main className={styles.content}>
          {error && <div className="error" role="alert">{error}</div>}
          <OperatorRole.Provider value={operator.role}>{children}</OperatorRole.Provider>
        </main>
      </div>
      {confirmingSignOut && (
        <div className={styles.modalBackdrop} role="presentation" onMouseDown={event => {
          if (event.target === event.currentTarget) setConfirmingSignOut(false);
        }}>
          <section className={styles.modal} role="dialog" aria-modal="true" aria-labelledby="signout-title" aria-describedby="signout-description">
            <div className={styles.modalIcon}><LogOut size={20} aria-hidden="true" /></div>
            <h2 id="signout-title">Sign out?</h2>
            <p id="signout-description">You will need to sign in again to access the central workspace.</p>
            <div className={styles.modalActions}>
              <button ref={cancelSignOutRef} className={styles.modalCancel} type="button" onClick={() => setConfirmingSignOut(false)}>
                Stay signed in
              </button>
              <button className={styles.modalConfirm} type="button" onClick={signOut} disabled={signingOut}>
                {signingOut ? 'Signing out…' : 'Sign out'}
              </button>
            </div>
          </section>
        </div>
      )}
      {profileOpen && <div className={styles.modalBackdrop} role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) setProfileOpen(false); }}>
        <section className={styles.modal} role="dialog" aria-modal="true" aria-labelledby="profile-title">
          <button className={styles.modalClose} type="button" aria-label="Close profile" onClick={() => setProfileOpen(false)}><X size={18} /></button>
          <div className={styles.modalIcon}><UserRound size={20} /></div>
          <h2 id="profile-title">Your profile</h2>
          <div className={styles.profileDetails}><span>Name</span><strong>{operator.name}</strong><span>Email</span><strong>{operator.email}</strong><span>Role</span><strong className={styles.roleValue}>{operator.role}</strong></div>
        </section>
      </div>}
      {passwordOpen && <div className={styles.modalBackdrop} role="presentation" onMouseDown={event => { if (event.target === event.currentTarget) setPasswordOpen(false); }}>
        <section className={styles.modal} role="dialog" aria-modal="true" aria-labelledby="password-title">
          <button className={styles.modalClose} type="button" aria-label="Close password form" onClick={() => setPasswordOpen(false)}><X size={18} /></button>
          <div className={styles.modalIcon}><KeyRound size={20} /></div>
          <h2 id="password-title">Change password</h2>
          <p>Use at least 12 characters. You will be signed out after the change.</p>
          {passwordError && <div className="error" role="alert">{passwordError}</div>}
          <form className={styles.passwordForm} onSubmit={changePassword}>
            <label>Current password<input type="password" required value={currentPassword} onChange={event => setCurrentPassword(event.target.value)} autoComplete="current-password" /></label>
            <label>New password<input type="password" required minLength={12} value={newPassword} onChange={event => setNewPassword(event.target.value)} autoComplete="new-password" /></label>
            <label>Confirm new password<input type="password" required minLength={12} value={confirmPassword} onChange={event => setConfirmPassword(event.target.value)} autoComplete="new-password" /></label>
            <button className={styles.modalConfirm} type="submit" disabled={changingPassword}>{changingPassword ? 'Updating…' : 'Update password'}</button>
          </form>
        </section>
      </div>}
    </div>
  );
}
