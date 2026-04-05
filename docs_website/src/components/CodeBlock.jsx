import { useState } from 'react'
import styles from './CodeBlock.module.css'

export default function CodeBlock({ children, title }) {
  const [copied, setCopied] = useState(false)

  const copy = () => {
    navigator.clipboard.writeText(children).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }

  return (
    <div className={styles.wrapper}>
      {title && <div className={styles.title}>{title}</div>}
      <div className={styles.container}>
        <button className={styles.copy} onClick={copy}>
          {copied ? 'Copied' : 'Copy'}
        </button>
        <pre className={styles.pre}>
          <code>{children}</code>
        </pre>
      </div>
    </div>
  )
}
