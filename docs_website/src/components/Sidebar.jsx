import { NavLink } from 'react-router'
import styles from './Sidebar.module.css'

const NAV_ITEMS = [
  { path: '/', label: 'Introduction' },
  { path: '/getting-started', label: 'Getting Started' },
  { path: '/configuration', label: 'Configuration' },
  { path: '/routing', label: 'Routing' },
  { path: '/rate-limiting', label: 'Rate Limiting' },
  { path: '/caching', label: 'Caching' },
  { path: '/load-balancing', label: 'Load Balancing' },
  { path: '/streaming', label: 'Streaming' },
  { path: '/deployment', label: 'Deployment' },
  { path: '/api-reference', label: 'API Reference' },
]

export default function Sidebar() {
  return (
    <nav className={styles.nav}>
      <div className={styles.brand}>
        <span className={styles.logo}>&#x26A1;</span>
        <span className={styles.title}>rpc-proxy</span>
      </div>
      <ul className={styles.list}>
        {NAV_ITEMS.map(({ path, label }) => (
          <li key={path}>
            <NavLink
              to={path}
              end={path === '/'}
              className={({ isActive }) =>
                `${styles.link} ${isActive ? styles.active : ''}`
              }
            >
              {label}
            </NavLink>
          </li>
        ))}
      </ul>
      <div className={styles.footer}>
        <a
          href="https://github.com/tezos-commons/rpc-proxy"
          target="_blank"
          rel="noopener noreferrer"
          className={styles.ghLink}
        >
          GitHub
        </a>
      </div>
    </nav>
  )
}
