"use client";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { findInFile } from "../lib/find-in-file";

export function TextFileEditor({ value, onChange, busy }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(null);
  const text = useRef(null);
  const search = useRef(null);
  const id = useId();
  const { matches, truncated } = useMemo(() => findInFile(value, query), [value, query]);
  const lines = useMemo(() => value.split("\n").length, [value]);
  const active = selected?.value === value && selected.query === query ? selected.index : -1;
  useEffect(() => {
    if (open) search.current?.focus();
  }, [open]);
  function close() {
    setOpen(false);
    setQuery("");
    setSelected(null);
    text.current?.focus();
  }
  function find() {
    if (open) {
      search.current?.focus();
      search.current?.select();
    } else setOpen(true);
  }
  function move(direction) {
    if (!matches.length) return;
    const index =
      active < 0
        ? direction > 0
          ? 0
          : matches.length - 1
        : (active + direction + matches.length) % matches.length;
    const match = matches[index];
    const line = value.slice(0, match.start).split("\n").length;
    const element = text.current;
    element.focus();
    element.setSelectionRange(match.start, match.end);
    const lineHeight = parseFloat(getComputedStyle(element).lineHeight) || 22;
    element.scrollTop = Math.max(0, (line - 1) * lineHeight - element.clientHeight / 2);
    setSelected({ index, line, value, query });
  }
  return (
    <div className="text-file-editor">
      <div className="file-editor-toolbar">
        <span className="field-help">
          {lines.toLocaleString()} {lines === 1 ? "line" : "lines"} · Plain text
        </span>
        <button
          type="button"
          className="button secondary"
          onClick={open ? close : find}
          title="Find text (Ctrl+F or ⌘F)"
          aria-expanded={open}
        >
          {open ? "Close search" : "Find in file"}
        </button>
      </div>
      {open && (
        <div className="file-findbar">
          <label>
            Find text in this file
            <input
              ref={search}
              type="search"
              value={query}
              maxLength={200}
              placeholder="Search settings or text…"
              onChange={(event) => {
                setQuery(event.target.value);
                setSelected(null);
              }}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault();
                  move(event.shiftKey ? -1 : 1);
                }
                if (event.key === "Escape") {
                  event.preventDefault();
                  close();
                }
              }}
            />
          </label>
          <div className="file-find-actions">
            <button
              type="button"
              className="button secondary"
              aria-label="Previous match"
              aria-controls={id}
              disabled={!matches.length}
              onClick={() => move(-1)}
            >
              Previous
            </button>
            <button
              type="button"
              className="button secondary"
              aria-label="Next match"
              aria-controls={id}
              disabled={!matches.length}
              onClick={() => move(1)}
            >
              Next
            </button>
          </div>
          <p className="field-help" aria-live="polite">
            {!query
              ? "Search stays in this file. Enter moves forward; Shift+Enter moves back."
              : !matches.length
                ? "No matches in this file."
                : active < 0
                  ? `${matches.length.toLocaleString()}${truncated ? "+" : ""} matches`
                  : `Match ${active + 1} of ${matches.length.toLocaleString()}${truncated ? "+" : ""} · line ${selected.line}`}
            {truncated && " · Narrow your search to see later matches."}
          </p>
        </div>
      )}
      <textarea
        id={id}
        ref={text}
        aria-label="File content"
        readOnly={busy}
        aria-busy={busy}
        className="file-editor"
        value={value}
        onChange={(event) => {
          setSelected(null);
          onChange(event.target.value);
        }}
        spellCheck={false}
        onKeyDown={(event) => {
          if (
            (event.ctrlKey || event.metaKey) &&
            !event.altKey &&
            !event.shiftKey &&
            event.key.toLowerCase() === "f"
          ) {
            event.preventDefault();
            find();
          }
        }}
      />
    </div>
  );
}
