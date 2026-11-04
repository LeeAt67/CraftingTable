import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { DatabaseSync } from "node:sqlite";

const migration = readFileSync(
  resolve(process.cwd(), "api/migrations/001Initial.sql"),
  "utf8",
);

export type LeeatDatabase = DatabaseSync;

export function openDatabase(filename: string): LeeatDatabase {
  const db = new DatabaseSync(filename);
  db.exec("PRAGMA foreign_keys = ON;");
  db.exec(migration);
  return db;
}
