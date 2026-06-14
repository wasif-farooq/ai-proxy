import { NavLink } from 'react-router-dom';
import { useAuth } from '../../hooks/useAuth';

const navItems = [
  { to: '/', label: 'Dashboard' },
  { to: '/clients', label: 'Clients' },
  { to: '/audit-logs', label: 'Audit Logs' },
  { to: '/settings', label: 'Settings' },
];

export const TopNav = () => {
  const { user, logout } = useAuth();

  return (
    <header className="h-20 bg-canvas border-b border-hairline flex items-center px-6 lg:px-10 sticky top-0 z-40">
      {/* Logo / brand */}
      <div className="flex items-center gap-2 shrink-0">
        <div className="w-8 h-8 rounded-full bg-primary flex items-center justify-center">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path d="M8 2l1.5 3L13 5.5l-2.5 2.5.75 3.5L8 9.5l-3 1.5.75-3.5L3 5.5 6.5 5 8 2z" fill="white" />
          </svg>
        </div>
        <span className="text-ink text-base font-[600] leading-[1.25] hidden sm:inline">
          AI Proxy
        </span>
      </div>

      {/* Nav links — centered */}
      <nav className="flex items-center gap-1 ml-8 lg:ml-12 overflow-x-auto">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) =>
              `px-3 py-2 text-base font-[600] leading-[1.25] whitespace-nowrap transition-colors duration-150 no-underline ${
                isActive
                  ? 'text-ink border-b-2 border-ink'
                  : 'text-muted hover:text-ink border-b-2 border-transparent'
              }`
            }
          >
            {item.label}
          </NavLink>
        ))}
      </nav>

      {/* Spacer */}
      <div className="flex-1" />

      {/* Account menu */}
      <div className="flex items-center gap-3 shrink-0">
        <span className="text-ink text-sm font-[400] leading-[1.43] hidden md:inline">
          {user?.name ?? 'Admin'}
        </span>
        <button
          onClick={logout}
          className="
            flex items-center justify-center
            w-8 h-8 rounded-full
            bg-surface-strong text-ink
            hover:bg-hairline
            transition-colors duration-150
            cursor-pointer border-none
          "
          aria-label="Log out"
          title="Log out"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path
              d="M6 14H3.333A1.333 1.333 0 012 12.667V3.333A1.333 1.333 0 013.333 2H6M10.667 11.333L14 8l-3.333-3.333M14 8H6"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </button>
      </div>
    </header>
  );
};
