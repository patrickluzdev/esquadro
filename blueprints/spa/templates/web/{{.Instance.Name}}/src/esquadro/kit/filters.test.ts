import { describe, expect, it } from "vitest"
import { cleared, filterKeys, isActive, isDateField } from "./filters.tsx"
import type { FieldMeta } from "./meta.ts"

const field = (name: string, kind: FieldMeta["kind"]) => ({ name, kind, label: name, required: false }) as FieldMeta

describe("filters", () => {
  it("keeps a range in two keys, the rest in one", () => {
    expect(filterKeys(field("price", "money"))).toEqual(["price_min", "price_max"])
    expect(filterKeys(field("created_at", "datetime"))).toEqual(["created_at_min", "created_at_max"])
    expect(filterKeys(field("status", "enum"))).toEqual(["status"])
    expect(isDateField(field("due", "date"))).toBe(true)
  })

  it("is set by any of its keys, not by an empty one", () => {
    expect(isActive(field("price", "money"), { price_max: "10" })).toBe(true)
    expect(isActive(field("status", "enum"), { status: "" })).toBe(false)
    expect(isActive(field("status", "enum"), {})).toBe(false)
  })

  it("clears every key of every filter", () => {
    expect(cleared([field("status", "enum"), field("price", "money")])).toEqual({ status: undefined, price_min: undefined, price_max: undefined })
  })
})
