import { readFileSync } from "node:fs";
import { DatabaseSync } from "node:sqlite";
import { fileURLToPath } from "node:url";

const migration = readFileSync(
  fileURLToPath(new URL("../../migrations/001Initial.sql", import.meta.url)),
  "utf8",
);

export type LeeatDatabase = DatabaseSync;

export function openDatabase(filename: string): LeeatDatabase {
  const db = new DatabaseSync(filename);
  db.exec("PRAGMA foreign_keys = ON;");
  db.exec(migration);
  return db;
}
