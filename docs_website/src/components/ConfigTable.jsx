import styles from './ConfigTable.module.css'

export default function ConfigTable({ rows }) {
  return (
    <div className={styles.wrapper}>
      <table className={styles.table}>
        <thead>
          <tr>
            <th>Field</th>
            <th>Type</th>
            <th>Default</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(({ field, type, def, desc }) => (
            <tr key={field}>
              <td><code>{field}</code></td>
              <td className={styles.type}>{type}</td>
              <td><code>{def}</code></td>
              <td>{desc}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
