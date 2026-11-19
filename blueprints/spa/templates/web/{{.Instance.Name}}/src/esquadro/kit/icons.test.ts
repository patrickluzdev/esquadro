import { describe, expect, it } from "vitest"
import { icons as yours } from "../../icons.tsx"
import { ChartColumn, Circle, iconOf, icons, lucideIcons, Trash2 } from "../icons.gen.ts"

describe("icons", () => {
  it("draws yours where src/icons.tsx gives one, Lucide's elsewhere", () => {
    for (const [name, icon] of Object.entries(lucideIcons)) {
      expect(icons[name]).toBe(yours[name] ?? icon)
    }
    expect(Trash2).toBe(icons.Trash2)
  })

  it("finds an icon by the name the model writes", () => {
    expect(iconOf("trash_2")).toBe(Trash2)
    expect(iconOf("trash-2")).toBe(Trash2)
    expect(iconOf("Trash2")).toBe(Trash2)
  })

  it("draws the fallback for a name the set lacks", () => {
    expect(iconOf("no_such_icon")).toBe(Circle)
    expect(iconOf(undefined, "ChartColumn")).toBe(ChartColumn)
  })
})
