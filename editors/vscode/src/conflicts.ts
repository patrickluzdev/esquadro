// The conflicts esquadro gen leaves in a file when a merge cannot settle
// them, marked as git marks them in its diff3 style:
//
//   <<<<<<< yours
//   (the user's lines)
//   ||||||| base
//   (the template's, as it wrote them last time)
//   =======
//   (the template's now)
//   >>>>>>> esquadro
//
// The file alone holds the three sides, which the merge editor shows.

const OURS = "<<<<<<< yours"
const BASE = "||||||| base"
const SPLIT = "======="
const THEIRS = ">>>>>>> esquadro"

/** A conflict: the 0-based line of its first marker, and of its last. */
export interface Conflict {
  start: number
  end: number
}

/** The file as each side has it: yours, the base and esquadro's. */
export interface Sides {
  yours: string
  base: string
  esquadro: string
  conflicts: Conflict[]
}

/** Splits text into lines, each with its line ending. */
function lines(text: string): string[] {
  return text.match(/[^\n]*\n|[^\n]+$/g) ?? []
}

const is = (line: string, marker: string) => line.replace(/\r?\n$/, "") === marker

/** The conflicts in a file, in order; none when it has no markers. */
export function findConflicts(text: string): Conflict[] {
  return sides(text).conflicts
}

/**
 * The three sides of a file with conflicts: outside them, the lines all
 * share; inside, each side's. A conflict without its base section (an
 * older esquadro's) has no base lines.
 */
export function sides(text: string): Sides {
  const out: Sides = { yours: "", base: "", esquadro: "", conflicts: [] }
  const all = lines(text)
  for (let i = 0; i < all.length; i++) {
    if (!is(all[i], OURS)) {
      out.yours += all[i]
      out.base += all[i]
      out.esquadro += all[i]
      continue
    }
    // Inside a conflict: the side each line goes to, until its end.
    let side: "yours" | "base" | "esquadro" = "yours"
    let j = i + 1
    let yours = ""
    let base = ""
    let esquadro = ""
    for (; j < all.length && !is(all[j], THEIRS); j++) {
      if (side === "yours" && is(all[j], BASE)) side = "base"
      else if (side !== "esquadro" && is(all[j], SPLIT)) side = "esquadro"
      else if (side === "yours") yours += all[j]
      else if (side === "base") base += all[j]
      else esquadro += all[j]
    }
    if (j === all.length) {
      // Not closed: not a conflict of esquadro's, kept as it is.
      for (const l of all.slice(i)) {
        out.yours += l
        out.base += l
        out.esquadro += l
      }
      break
    }
    out.yours += yours
    out.base += base
    out.esquadro += esquadro
    out.conflicts.push({ start: i, end: j })
    i = j
  }
  return out
}
