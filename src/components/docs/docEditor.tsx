import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import { Placeholder } from "@tiptap/extensions";
import { Markdown } from "@tiptap/markdown";
import StarterKit from "@tiptap/starter-kit";
import { Bold, Code, Italic, Link2, List, ListOrdered, Quote, Strikethrough } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/components/utils";

type Props = {
  documentId: string;
  initialMarkdown: string;
  onChange: (markdown: string) => void;
  ariaLabel?: string;
};

export function DocEditor({
  documentId,
  initialMarkdown,
  onChange,
  ariaLabel = "文档正文",
}: Props) {
  const editor = useEditor(
    {
      extensions: [
        StarterKit,
        Markdown,
        Placeholder.configure({ placeholder: "开始写点什么…" }),
      ],
      content: initialMarkdown,
      contentType: "markdown",
      editorProps: {
        attributes: {
          class: "doc-prose",
          "aria-label": ariaLabel,
        },
      },
      onUpdate: ({ editor: instance }) => {
        onChange(instance.getMarkdown());
      },
    },
    [documentId],
  );

  const state = useEditorState({
    editor,
    selector: ({ editor: instance }) => {
      if (!instance) {
        return null;
      }
      return {
        bold: instance.isActive("bold"),
        italic: instance.isActive("italic"),
        strike: instance.isActive("strike"),
        code: instance.isActive("code"),
        link: instance.isActive("link"),
        bullet: instance.isActive("bulletList"),
        ordered: instance.isActive("orderedList"),
        quote: instance.isActive("blockquote"),
      };
    },
  });

  if (!editor) {
    return null;
  }

  const toggleLink = () => {
    if (editor.isActive("link")) {
      editor.chain().focus().unsetLink().run();
      return;
    }
    const href = window.prompt("输入链接地址", "https://");
    if (!href) {
      return;
    }
    editor.chain().focus().extendMarkRange("link").setLink({ href }).run();
  };

  const items = [
    {
      label: "加粗",
      icon: Bold,
      active: state?.bold,
      run: () => editor.chain().focus().toggleBold().run(),
    },
    {
      label: "斜体",
      icon: Italic,
      active: state?.italic,
      run: () => editor.chain().focus().toggleItalic().run(),
    },
    {
      label: "删除线",
      icon: Strikethrough,
      active: state?.strike,
      run: () => editor.chain().focus().toggleStrike().run(),
    },
    {
      label: "行内代码",
      icon: Code,
      active: state?.code,
      run: () => editor.chain().focus().toggleCode().run(),
    },
    {
      label: "链接",
      icon: Link2,
      active: state?.link,
      run: toggleLink,
    },
    {
      label: "无序列表",
      icon: List,
      active: state?.bullet,
      run: () => editor.chain().focus().toggleBulletList().run(),
    },
    {
      label: "有序列表",
      icon: ListOrdered,
      active: state?.ordered,
      run: () => editor.chain().focus().toggleOrderedList().run(),
    },
    {
      label: "引用",
      icon: Quote,
      active: state?.quote,
      run: () => editor.chain().focus().toggleBlockquote().run(),
    },
  ];

  return (
    <div className="relative">
      <BubbleMenu
        editor={editor}
        options={{ placement: "top" }}
        shouldShow={({ from, to }) => from !== to}
      >
        <div className="flex items-center gap-0.5 rounded-[10px] border border-border bg-surface p-1 shadow-lg">
          {items.map((item) => (
            <Button
              key={item.label}
              type="button"
              variant="ghost"
              size="icon"
              aria-label={item.label}
              title={item.label}
              aria-pressed={item.active}
              onClick={item.run}
              className={cn(
                "size-8 text-textSecondary hover:text-textPrimary",
                item.active && "bg-brandSoft text-brand",
              )}
            >
              <item.icon aria-hidden="true" size={15} />
            </Button>
          ))}
        </div>
      </BubbleMenu>
      <EditorContent editor={editor} />
    </div>
  );
}
