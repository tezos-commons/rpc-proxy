import styles from './Callout.module.css'

const icons = {
  info: '\u2139\uFE0F',
  warning: '\u26A0\uFE0F',
  tip: '\uD83D\uDCA1',
}

export default function Callout({ variant = 'info', children }) {
  return (
    <div className={`${styles.callout} ${styles[variant]}`}>
      <span className={styles.icon}>{icons[variant]}</span>
      <div className={styles.body}>{children}</div>
    </div>
  )
}
