import { strict as assert } from "node:assert"
import { test } from "node:test"
import { findConflicts, sides } from "./conflicts"

const merged = [
  "a",
  "<<<<<<< yours",
  "mine",
  "||||||| base",
  "b",
  "=======",
  "esquadro's",
  ">>>>>>> esquadro",
  "c",
  "",
].join("\n")

test("a conflict's three sides", () => {
  const s = sides(merged)
  assert.equal(s.yours, "a\nmine\nc\n")
  assert.equal(s.base, "a\nb\nc\n")
  assert.equal(s.esquadro, "a\nesquadro's\nc\n")
  assert.deepEqual(s.conflicts, [{ start: 1, end: 7 }])
})

test("a conflict without its base", () => {
  const s = sides("<<<<<<< yours\nmine\n=======\ntheirs\n>>>>>>> esquadro\n")
  assert.equal(s.yours, "mine\n")
  assert.equal(s.base, "")
  assert.equal(s.esquadro, "theirs\n")
})

test("no conflicts, or one not closed", () => {
  assert.deepEqual(findConflicts("a\nb\n"), [])
  assert.deepEqual(findConflicts("<<<<<<< yours\nmine\n"), [])
  assert.equal(sides("<<<<<<< yours\nmine\n").yours, "<<<<<<< yours\nmine\n")
})

test("Windows line endings", () => {
  const s = sides(merged.replaceAll("\n", "\r\n"))
  assert.equal(s.esquadro, "a\r\nesquadro's\r\nc\r\n")
  assert.equal(s.conflicts.length, 1)
})
