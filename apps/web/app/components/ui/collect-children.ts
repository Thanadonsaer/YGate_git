import { Children, isValidElement, type ReactElement, type ReactNode } from "react";

export function collectByType<P>(children: ReactNode, type: unknown): ReactElement<P>[] {
  const found: ReactElement<P>[] = [];
  Children.forEach(children, (child) => {
    if (!isValidElement(child)) return;
    if (child.type === type) {
      found.push(child as ReactElement<P>);
      return;
    }
    const nested = (child.props as { children?: ReactNode } | null)?.children;
    if (nested) found.push(...collectByType<P>(nested, type));
  });
  return found;
}
