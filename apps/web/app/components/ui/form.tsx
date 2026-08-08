"use client";

import * as React from "react";
import { Checkbox as PrimeCheckbox } from "primereact/checkbox";
import { InputText } from "primereact/inputtext";
import { InputTextarea } from "primereact/inputtextarea";
import { Message } from "primereact/message";
import { Password } from "primereact/password";
import { Tag } from "primereact/tag";
import { Eye, EyeOff } from "lucide-react";
import { cn } from "../../lib/cn";

// PrimeReact runs unstyled app-wide (see PrimeReactProvider in layout.tsx), so
// these primitives render the same DOM the hand-written markup did and keep
// inheriting globals.css -- the component library supplies behaviour
// (masking, checked state, ARIA wiring), the stylesheet supplies the looks.

/** Drop-in for `<input>`. */
export { InputText as TextInput };
/** Drop-in for `<textarea>`. */
export { InputTextarea as TextArea };

/** `<input type="password">` plus PrimeReact's built-in show/hide toggle. */
export function PasswordInput({
  className,
  inputClassName,
  ...rest
}: React.ComponentProps<typeof Password>) {
  return (
    <Password
      toggleMask
      feedback={false}
      // These MUST stay functions. PrimeReact builds the toggle's props
      // (onClick/onKeyDown/role/tabIndex/aria) then calls IconUtils.getJSXIcon
      // -> ObjectUtils.getJSXElement, which only applies them when the icon is
      // a function -- a bare JSX element is returned verbatim, silently
      // dropping onClick, so the eye renders but clicking never unmasks. The
      // <span> is what `.password-field > div > span` positions against input.
      showIcon={(options) => <span {...(options.iconProps as React.HTMLAttributes<HTMLSpanElement>)}><Eye size={18} /></span>}
      hideIcon={(options) => <span {...(options.iconProps as React.HTMLAttributes<HTMLSpanElement>)}><EyeOff size={18} /></span>}
      // PrimeReact forwards `pt.iconField`/`pt.inputIcon` into nested
      // components rather than applying them here, so the wrapper and the
      // toggle are positioned from globals.css (`.password-field`) instead.
      pt={{
        root: { className: cn("password-field", className) },
        input: { className: inputClassName },
      }}
      {...rest}
    />
  );
}

/** `<input type="checkbox">` with a real box element so it can be styled. */
export function Checkbox({
  checked,
  onChange,
  disabled,
  className,
  ...rest
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
}) {
  return (
    <PrimeCheckbox
      checked={checked}
      disabled={disabled}
      onChange={(event) => onChange(Boolean(event.checked))}
      pt={{
        root: { className: cn("relative inline-flex h-[18px] w-[18px] shrink-0 align-middle", className) },
        input: { className: "absolute inset-0 z-10 m-0 h-full w-full cursor-pointer opacity-0" },
        box: (options: { context: { checked: boolean; disabled: boolean } }) => ({
          className: cn(
            "grid h-[18px] w-[18px] place-items-center rounded-[4px] border transition",
            options.context.checked ? "border-brand bg-brand text-white" : "border-line bg-surface",
            options.context.disabled && "opacity-48",
          ),
        }),
        icon: { className: "h-3 w-3" },
      }}
      {...rest}
    />
  );
}

/** Inline form feedback -- replaces the hand-rolled `<p class="form-message">`. */
export function FormMessage({
  severity = "error",
  className,
  children,
}: {
  severity?: "error" | "success" | "warn" | "info";
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <Message
      severity={severity}
      text={children}
      pt={{
        root: { className: cn("form-message", severity, className) },
        icon: { className: "form-message-icon" },
        text: { className: "min-w-0 flex-1" },
      }}
    />
  );
}

/** Status pill (`ใช้งาน` / `Online` / ...). Keeps the existing `.status.*` skin. */
export function StatusTag({
  tone,
  className,
  children,
}: {
  /** Maps to the `.status.<tone>` rule in globals.css. */
  tone: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <Tag
      pt={{ root: { className: cn("status", tone, className) }, value: { className: "contents" } }}
      value={children}
    />
  );
}
