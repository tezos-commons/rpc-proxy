import useHeadings from '../hooks/useHeadings'
import useActiveHeading from '../hooks/useActiveHeading'
import styles from './TableOfContents.module.css'

export default function TableOfContents() {
  const headings = useHeadings()
  const activeId = useActiveHeading(headings)

  if (headings.length === 0) return null

  return (
    <nav className={styles.toc}>
      <div className={styles.title}>On this page</div>
      <ul className={styles.list}>
        {headings.map(({ id, text, level }) => (
          <li key={id}>
            <a
              href={`#${id}`}
              className={`${styles.link} ${level === 3 ? styles.nested : ''} ${
                activeId === id ? styles.active : ''
              }`}
            >
              {text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
