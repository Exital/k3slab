import type { Components } from "react-markdown";
import ReactMarkdown from "react-markdown";

/** Shared link styling for workshop / lab markdown. */
const linkClassName =
  "font-medium text-teal-700 underline decoration-teal-600/70 decoration-2 underline-offset-[3px] transition-colors hover:text-teal-950 hover:decoration-teal-800 dark:text-cyan-400 dark:decoration-cyan-400/60 dark:hover:text-cyan-300 dark:hover:decoration-cyan-300";

export const markdownComponents: Components = {
  a: ({ href, children, className, ...rest }) => {
    const isExternal =
      typeof href === "string" &&
      (href.startsWith("http://") ||
        href.startsWith("https://") ||
        href.startsWith("//"));
    return (
      <a
        href={href}
        className={className ? `${linkClassName} ${className}` : linkClassName}
        {...(isExternal ? { target: "_blank", rel: "noopener noreferrer" } : {})}
        {...rest}
      >
        {children}
      </a>
    );
  },
};

type MarkdownContentProps = {
  children: string;
  className?: string;
};

export function MarkdownContent({ children, className }: MarkdownContentProps) {
  if (!className) {
    return (
      <ReactMarkdown components={markdownComponents}>{children}</ReactMarkdown>
    );
  }
  return (
    <div className={className}>
      <ReactMarkdown components={markdownComponents}>{children}</ReactMarkdown>
    </div>
  );
}
