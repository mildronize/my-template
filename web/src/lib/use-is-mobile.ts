import { useEffect, useState } from "react";

/**
 * Returns true when the viewport width is below 640px (Tailwind `sm` breakpoint).
 *
 * SSR-safe: defaults to `false` on the server and during the first render on
 * the client, then resolves to the real value after mount. This avoids any
 * hydration mismatch because the server and the first client paint always agree
 * on `false`, and the layout effect fires only in the browser.
 */
export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState(false);

  useEffect(() => {
    const mq = window.matchMedia("(max-width: 639px)");
    setIsMobile(mq.matches);

    function handleChange(e: MediaQueryListEvent) {
      setIsMobile(e.matches);
    }

    mq.addEventListener("change", handleChange);
    return () => mq.removeEventListener("change", handleChange);
  }, []);

  return isMobile;
}
