import { useState, useEffect } from 'react'

export default function useActiveHeading(headings) {
  const [activeId, setActiveId] = useState('')

  useEffect(() => {
    if (headings.length === 0) return

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setActiveId(entry.target.id)
          }
        }
      },
      { rootMargin: '-80px 0px -70% 0px', threshold: 0 }
    )

    const els = headings.map((h) => document.getElementById(h.id)).filter(Boolean)
    els.forEach((el) => observer.observe(el))

    return () => observer.disconnect()
  }, [headings])

  return activeId
}
