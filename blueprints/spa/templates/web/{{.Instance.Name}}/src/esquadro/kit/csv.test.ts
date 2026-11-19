import { describe, expect, it } from "vitest"
import { parseCsv, parseDate, parseDateTime, parseNumber, problemsCsv, readRows, suggest } from "./csv.ts"
import { toCsv } from "./export.ts"
import type { FieldMeta } from "./meta.ts"

const words = { yes: "Sim", no: "Não" }

describe("parseCsv", () => {
  it("reads ; and , files, quotes, line breaks and a byte-order mark", () => {
    const semi = parseCsv('﻿Nome;Valor\r\n"Silva; João";"1.234,56"\r\n"Diz ""oi""\nem duas";2\r\n\r\n')
    expect(semi.separator).toBe(";")
    expect(semi.header).toEqual(["Nome", "Valor"])
    expect(semi.rows).toEqual([
      ["Silva; João", "1.234,56"],
      ['Diz "oi"\nem duas', "2"],
    ])
    const comma = parseCsv("name,price\nPen,1.50")
    expect(comma.separator).toBe(",")
    expect(comma.rows).toEqual([["Pen", "1.50"]])
  })
})

describe("parseNumber", () => {
  it("reads numbers as the language writes them, and as code does", () => {
    expect(parseNumber("1.234,56", "pt-BR")).toBe("1234.56")
    expect(parseNumber("R$ 1.234,56", "pt-BR")).toBe("1234.56")
    expect(parseNumber("1.500", "pt-BR")).toBe("1500")
    expect(parseNumber("1.5", "pt-BR")).toBe("1.5")
    expect(parseNumber("1234.56", "pt-BR")).toBe("1234.56")
    expect(parseNumber("1,234.56", "en")).toBe("1234.56")
    expect(parseNumber("-3", "en")).toBe("-3")
    expect(parseNumber("abc", "en")).toBeNull()
  })
})

describe("parseDate", () => {
  it("reads ISO, numeric and named-month dates in the language", () => {
    expect(parseDate("2026-11-18", "pt-BR")).toBe("2026-11-18")
    expect(parseDate("18/11/2026", "pt-BR")).toBe("2026-11-18")
    expect(parseDate("11/18/2026", "en")).toBe("2026-11-18")
    expect(parseDate("18 de nov. de 2026", "pt-BR")).toBe("2026-11-18")
    expect(parseDate("Nov 18, 2026", "en")).toBe("2026-11-18")
    expect(parseDate("31/02/2026", "pt-BR")).toBeNull()
    expect(parseDate("soon", "en")).toBeNull()
  })
  it("reads a moment with its time, in the browser's zone", () => {
    expect(parseDateTime("18/11/2026 14:30", "pt-BR")).toBe(new Date(2026, 10, 18, 14, 30).toISOString())
    expect(parseDateTime("2026-11-18T10:00:00Z", "en")).toBe("2026-11-18T10:00:00.000Z")
  })
})

const fields: FieldMeta[] = [
  { name: "name", label: "Nome", kind: "text", required: true },
  { name: "price", label: "Preço", kind: "money", required: true },
  { name: "status", label: "Situação", kind: "enum", required: false, values: ["ACTIVE", "INACTIVE"], labels: { ACTIVE: "Ativo", INACTIVE: "Inativo" }, default: "ACTIVE" },
  { name: "open", label: "Aberto", kind: "bool", required: false },
  { name: "category_id", label: "Categoria", kind: "relation", required: false, target: "categories", display: "name" },
]

describe("suggest", () => {
  it("gives each column the field of its label or name, once", () => {
    expect(suggest(["NOME", "Preço", "Preço (Moeda)", "situacao", "categoria", "Nome", "outra"], fields, "Moeda")).toEqual([
      "name",
      "price",
      "price#currency",
      "status",
      "category_id",
      null,
      null,
    ])
    expect(suggest(["Nome", "Título"], fields, "Moeda", { titulo: "name", nome: "" })).toEqual([null, "name"])
  })
})

describe("readRows", () => {
  const lookups = { category_id: new Map([["livros", ["c1"]], ["casa", ["c2", "c3"]]]) }
  const options = { locale: "pt-BR", currency: "BRL", words, lookups }

  it("reads values in the language, with defaults and relations by name", () => {
    const csv = parseCsv("Nome;Preço;Preço (Moeda);Situação;Aberto;Categoria\nCaneta;1.234,56;USD;Inativo;sim;Livros\nLápis;2,5;;;não;")
    const { records, problems } = readRows(csv, suggest(csv.header, fields, "Moeda"), fields, options)
    expect(problems).toEqual([])
    expect(records).toEqual([
      { name: "Caneta", price: { amount: "1234.56", currency: "USD" }, status: "INACTIVE", open: true, category_id: "c1" },
      { name: "Lápis", price: { amount: "2.5", currency: "BRL" }, status: "ACTIVE", open: false },
    ])
  })

  it("says what is wrong in each row", () => {
    const csv = parseCsv("Nome;Preço;Situação;Categoria\n;abc;Talvez;Casa\nOk;1;;Nenhuma")
    const { problems } = readRows(csv, suggest(csv.header, fields, "Moeda"), fields, options)
    expect(problems).toEqual([
      { row: 1, field: "price", code: "invalid_value" },
      { row: 1, field: "status", code: "invalid_value" },
      { row: 1, field: "category_id", code: "ambiguous" },
      { row: 1, field: "name", code: "required" },
      { row: 2, field: "category_id", code: "not_found" },
    ])
    expect(problemsCsv(csv, problems, (p) => `${p.field}: ${p.code}`, "Erro").split("\r\n")[1]).toBe(
      ';abc;Talvez;Casa;"price: invalid_value; status: invalid_value; category_id: ambiguous; name: required"',
    )
  })

  it("imports what the export wrote", () => {
    const exported = toCsv(
      fields.filter((f) => f.kind !== "relation").map((f) => ({ field: f, label: f.label })),
      [{ id: "1", version: 1, name: "Caneta", price: { amount: "1234.56", currency: "BRL" }, status: "INACTIVE", open: true }],
      ";",
    )
    const csv = parseCsv(exported)
    const { records, problems } = readRows(csv, suggest(csv.header, fields, "Moeda"), fields, options)
    expect(problems).toEqual([])
    expect(records[0]).toEqual({ name: "Caneta", price: { amount: "1234.56", currency: "BRL" }, status: "INACTIVE", open: true })
  })
})
