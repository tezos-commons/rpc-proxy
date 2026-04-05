import { useState, useEffect } from 'react'
import { useLocation } from 'react-router'

export default function useHeadings() {
  const [headings, setHeadings] = useState([])
  const location = useLocation()

  useEffect(() => {
    const timer = requestAnimationFrame(() => {
      const els = document.querySelectorAll('.page h2[id], .page h3[id]')
      setHeadings(
        Array.from(els).map((el) => ({
          id: el.id,
          text: el.textContent,
          level: el.tagName === 'H3' ? 3 : 2,
        }))
      )
    })
    return () => cancelAnimationFrame(timer)
  }, [location.pathname])

  return headings
}
