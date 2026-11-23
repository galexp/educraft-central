'use client';import {useOperatorRole} from '@/components/Shell';import {Crud} from '@/components/Crud';
import styles from '@/components/Records.module.css';

export default function Users() {
  const canManage = ['owner', 'admin'].includes(useOperatorRole());
  if (!canManage) {
    return <p className="panel">Operator management is restricted to owners and admins.</p>;
  }

  return (
    <section className={styles.records}>
      <Crud
        title="Operators"
        subtitle="Manage staff accounts and workspace roles"
        path="/operators"
        cols={[
          {key: 'name', label: 'Name'},
          {key: 'email', label: 'Email'},
          {key: 'role', label: 'Role', format: 'status'},
          {key: 'active', label: 'Active'},
          {key: 'created_at', label: 'Created', format: 'date'},
        ]}
        fields={[
          {name: 'name', label: 'Full name', required: true},
          {name: 'email', label: 'Email', type: 'email', required: true},
          {name: 'password', label: 'Temporary password (optional; 12+ characters)', type: 'password'},
          {name: 'role', label: 'Role', type: 'select', required: true,
            options: ['admin', 'finance', 'support', 'viewer'].map(role => ({value: role, label: role}))},
        ]}
      />
    </section>
  );
}
