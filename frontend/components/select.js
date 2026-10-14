"use client";

import { Children, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";

// One control for every dashboard dropdown, including keyboard and touch users.
export function Select({ children, value, onChange, disabled, name, required, ...props }) {
  const options = Children.toArray(children).flatMap((child) =>
    child.type === "option" ? [child] : Children.toArray(child.props?.children || []),
  );
  const selected = options.findIndex((option) => String(option.props.value) === String(value));
  const [menu, setMenu] = useState(null);
  const [night, setNight] = useState(false);
  const [active, setActive] = useState(0);
  const trigger = useRef(null);
  const popup = useRef(null);
  const typed = useRef({ text: "", at: 0 });
  const id = useId();
  function position() {
    const rect = trigger.current.getBoundingClientRect();
    const below = window.innerHeight - rect.bottom - 12;
    const above = rect.top - 12;
    return {
      left: Math.max(8, Math.min(rect.left, window.innerWidth - rect.width - 8)),
      width: Math.min(rect.width, window.innerWidth - 16),
      maxHeight: Math.min(300, Math.max(below, above)),
      ...(below >= Math.min(300, above)
        ? { top: rect.bottom + 6 }
        : { bottom: window.innerHeight - rect.top + 6 }),
    };
  }
  function open() {
    if (disabled) return;
    setNight(Boolean(trigger.current.closest(".app-shell, .public-player-page, .auth-page")));
    setMenu(position());
    setActive(Math.max(0, selected));
  }
  function choose(index) {
    const option = options[index];
    if (!option || option.props.disabled) return;
    onChange?.({ target: { value: String(option.props.value), name } });
    setMenu(null);
    trigger.current.focus();
  }
  useEffect(() => {
    if (!menu) return;
    const close = (event) => {
      if (!trigger.current?.contains(event.target) && !popup.current?.contains(event.target))
        setMenu(null);
    };
    const reposition = (event) => {
      if (!popup.current?.contains(event.target)) setMenu(position());
    };
    document.addEventListener("pointerdown", close);
    window.addEventListener("resize", reposition);
    window.addEventListener("scroll", reposition, true);
    return () => {
      document.removeEventListener("pointerdown", close);
      window.removeEventListener("resize", reposition);
      window.removeEventListener("scroll", reposition, true);
    };
  }, [menu]);
  useEffect(() => {
    const list = popup.current;
    const item = list?.querySelector(`[data-index="${active}"]`);
    if (!item) return;
    if (item.offsetTop < list.scrollTop) list.scrollTop = item.offsetTop;
    else if (item.offsetTop + item.offsetHeight > list.scrollTop + list.clientHeight)
      list.scrollTop = item.offsetTop + item.offsetHeight - list.clientHeight;
  }, [active, menu]);
  function keyboard(event) {
    if (event.key === "Tab" || event.key === "Escape") {
      setMenu(null);
      return;
    }
    if (["ArrowDown", "ArrowUp", "Home", "End", "Enter", " "].includes(event.key)) {
      event.preventDefault();
      if (!menu) return open();
      if (event.key === "Enter" || event.key === " ") return choose(active);
      let next = event.key === "Home" ? 0 : event.key === "End" ? options.length - 1 : active;
      const direction = event.key === "ArrowUp" || event.key === "End" ? -1 : 1;
      if (event.key.startsWith("Arrow")) next += direction;
      for (let n = 0; n < options.length; n++, next += direction) {
        const index = (next + options.length) % options.length;
        if (!options[index].props.disabled) return setActive(index);
      }
    } else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey) {
      if (!menu) open();
      const now = Date.now();
      typed.current = {
        text: (now - typed.current.at < 700 ? typed.current.text : "") + event.key.toLowerCase(),
        at: now,
      };
      const match = options.findIndex(
        (option) =>
          !option.props.disabled &&
          String(option.props.children).toLowerCase().startsWith(typed.current.text),
      );
      if (match >= 0) setActive(match);
    }
  }
  return (
    <>
      {name && <input type="hidden" name={name} value={value ?? ""} disabled={disabled} />}
      <button
        {...props}
        type="button"
        ref={trigger}
        className={`select-trigger ${props.className || ""}`}
        disabled={disabled}
        data-value={value ?? ""}
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={Boolean(menu)}
        aria-controls={menu ? id : undefined}
        aria-required={required || undefined}
        aria-activedescendant={menu ? `${id}-${active}` : undefined}
        onClick={() => (menu ? setMenu(null) : open())}
        onKeyDown={keyboard}
      >
        <span>{options[selected]?.props.children || "Choose an option"}</span>
        <svg viewBox="0 0 20 20" width="18" height="18" aria-hidden="true">
          <path
            d="m5 8 5 5 5-5"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.7"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </button>
      {menu &&
        createPortal(
          <div
            ref={popup}
            id={id}
            role="listbox"
            className={`select-menu${night ? " select-menu-night" : ""}`}
            style={menu}
            aria-label={props["aria-label"]}
          >
            {options.map((option, index) => (
              <div
                key={option.props.value}
                id={`${id}-${index}`}
                role="option"
                data-value={option.props.value}
                data-index={index}
                aria-selected={index === selected}
                aria-disabled={Boolean(option.props.disabled)}
                className={`select-option ${index === active ? "active" : ""}`}
                onPointerMove={() => setActive(index)}
                onPointerDown={(event) => event.preventDefault()}
                onClick={() => choose(index)}
              >
                <span>{option.props.children}</span>
                {index === selected && <span aria-hidden="true">✓</span>}
              </div>
            ))}
          </div>,
          trigger.current.closest("dialog") || document.body,
        )}
    </>
  );
}
