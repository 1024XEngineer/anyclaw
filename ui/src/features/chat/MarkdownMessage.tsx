import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

type MarkdownMessageProps = {
  content: string;
};

export function MarkdownMessage({ content }: MarkdownMessageProps) {
  return (
    <div className="chat-markdown">
      <ReactMarkdown
        components={{
          a: ({ node: _node, ...props }) => (
            <a {...props} rel="noreferrer" target="_blank" />
          ),
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
