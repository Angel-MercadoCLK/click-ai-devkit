// TOML values for Codex's `-c key=value` flag, written so they survive a Windows `.cmd` shim.
//
// Codex is usually a `.cmd` launcher on Windows, and cmd.exe re-reads the command line: a space, a double quote or one of
// `%&|<>^!` changes its meaning. Every override Dot passes is therefore a TOML *literal* string ('single quotes', no
// escapes) made only of characters that are safe in both places. A value that cannot be written that way is refused
// instead of being quoted into something cmd.exe could reinterpret.

/** The characters a literal value may hold: letters, digits and `_ . - : / @`. No spaces, quotes or shell metacharacters. */
const SAFE_VALUE = /^[A-Za-z0-9_.\-:/@]+$/;

export function tomlLiteral(value: string): string {
  if (!SAFE_VALUE.test(value)) throw new Error("The value cannot be passed to Codex safely as a config override.");
  return `'${value}'`;
}

export function tomlLiteralArray(values: readonly string[]): string {
  return `[${values.map(tomlLiteral).join(",")}]`;
}
