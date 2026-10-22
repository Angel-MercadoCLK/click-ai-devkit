// The page's side of the cursor feed: the desktop shell polls the cursor (it is the only process that can read it
// anywhere on the screen) and announces each position with this DOM event. `desktop/cursor.mts` holds the shell's copy of
// the name; a test keeps the two equal.

export const CURSOR_EVENT = "click-dot:cursor";
