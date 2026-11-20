import { describe, expect, it } from "vitest"
import { columns } from "./form.tsx"
import type { FormSection } from "./resource.ts"

describe("the form's columns", () => {
  const details: FormSection = { title: "Details", fields: [] }
  const preview: FormSection = { title: "Preview", side: true, render: () => null }

  it("puts the side's cards at the right", () => {
    expect(columns([details, preview])).toEqual({ main: [details], side: [preview] })
  })

  it("puts them in the main column when there is no other", () => {
    expect(columns([preview])).toEqual({ main: [preview], side: [] })
  })

  it("takes a card with no fields, only what it renders", () => {
    expect(preview.fields).toBeUndefined()
    expect(columns([preview, details]).side).toEqual([preview])
  })
})
