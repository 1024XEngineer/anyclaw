import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

type MarkdownMessageProps = {
  content: string;
};

function getSafeMarkdownHref(href?: string) {
  const candidate = href?.trim();
  if (!candidate) {
    return null;
  }

  try {
    const parsed = new URL(candidate);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return null;
    }

    return candidate;
  } catch {
    return null;
  }
}

export function MarkdownMessage({ content }: MarkdownMessageProps) {
  return (
    <div className="chat-markdown">
      <ReactMarkdown
        components={{
          a: ({ node: _node, href, children, ...props }) => {
            const safeHref = getSafeMarkdownHref(href);
            if (!safeHref) {
              return <span>{children}</span>;
            }

            return (
              <a {...props} href={safeHref} rel="noreferrer" target="_blank">
                {children}
              </a>
            );
          },
          table: ({ node: _node, ...props }) => (
            <div className="overflow-x-auto">
              <table {...props} />
            </div>
          ),
        }}
        remarkPlugins={[remarkGfm]}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}
