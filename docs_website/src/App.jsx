import { Routes, Route, Navigate, useLocation } from 'react-router'
import { useState, useEffect } from 'react'
import Sidebar from './components/Sidebar'
import TableOfContents from './components/TableOfContents'
import Introduction from './pages/Introduction'
import GettingStarted from './pages/GettingStarted'
import Configuration from './pages/Configuration'
import Routing from './pages/Routing'
import RateLimiting from './pages/RateLimiting'
import Caching from './pages/Caching'
import LoadBalancing from './pages/LoadBalancing'
import Streaming from './pages/Streaming'
import Deployment from './pages/Deployment'
import ApiReference from './pages/ApiReference'
import styles from './App.module.css'

function App() {
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const location = useLocation()

  useEffect(() => {
    setMobileNavOpen(false)
    window.scrollTo(0, 0)
  }, [location.pathname])

  return (
    <div className={styles.layout}>
      <button
        className={styles.hamburger}
        onClick={() => setMobileNavOpen(!mobileNavOpen)}
        aria-label="Toggle navigation"
      >
        <span />
        <span />
        <span />
      </button>

      {mobileNavOpen && (
        <div className={styles.overlay} onClick={() => setMobileNavOpen(false)} />
      )}

      <aside className={`${styles.sidebar} ${mobileNavOpen ? styles.sidebarOpen : ''}`}>
        <Sidebar />
      </aside>

      <main className={`${styles.content} page`}>
        <Routes>
          <Route path="/" element={<Introduction />} />
          <Route path="/getting-started" element={<GettingStarted />} />
          <Route path="/configuration" element={<Configuration />} />
          <Route path="/routing" element={<Routing />} />
          <Route path="/rate-limiting" element={<RateLimiting />} />
          <Route path="/caching" element={<Caching />} />
          <Route path="/load-balancing" element={<LoadBalancing />} />
          <Route path="/streaming" element={<Streaming />} />
          <Route path="/deployment" element={<Deployment />} />
          <Route path="/api-reference" element={<ApiReference />} />
          <Route path="*" element={<Navigate to="/" />} />
        </Routes>
      </main>

      <aside className={styles.toc}>
        <TableOfContents />
      </aside>
    </div>
  )
}

export default App
