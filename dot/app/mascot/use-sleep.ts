"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { sleepStatus } from "./orbi-look.ts";

/**
 * Whether Orbi is asleep: after about three minutes in which nothing is going on (`quiet`), until anything wakes it. `wake`
 * is for any activity (the pointer on the island, a click, a key); it only notes the time, so calling it on every pointer
 * move costs nothing. One timer, set to the moment it would fall asleep, and none while something is going on.
 */
export function useSleep({ quiet }: { quiet: boolean }) {
  const [asleep, setAsleep] = useState(false);
  const lastActivity = useRef(Date.now());

  const wake = useCallback(() => {
    lastActivity.current = Date.now();
    setAsleep(false);
  }, []);

  const wasQuiet = useRef(false);

  useEffect(() => {
    if (!quiet) {
      wasQuiet.current = false;
      setAsleep(false);
      return;
    }
    // The quiet starts when the last thing going on ends, not when it began.
    if (!wasQuiet.current) lastActivity.current = Date.now();
    wasQuiet.current = true;
    // Once asleep there is nothing to wait for until something wakes it.
    if (asleep) return;
    let timer: number | undefined;
    const check = () => {
      const status = sleepStatus({ quiet: true, lastActivityAt: lastActivity.current, now: Date.now() });
      if (status.asleep) setAsleep(true);
      else if (status.checkInMs !== null) timer = window.setTimeout(check, status.checkInMs);
    };
    check();
    return () => window.clearTimeout(timer);
  }, [quiet, asleep]);

  return { asleep, wake };
}
