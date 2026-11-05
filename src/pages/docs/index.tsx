import { FileText } from "lucide-react";

export default function DocsEmptyPage() {
  return (
    <section className="grid h-full place-items-center bg-surface px-6">
      <div className="text-center">
        <div className="mx-auto grid size-12 place-items-center rounded-xl bg-brandSoft text-brand">
          <FileText aria-hidden="true" size={20} />
        </div>
        <h1 className="mt-4 text-base font-semibold text-textPrimary">
          选择或新建一篇文档
        </h1>
        <p className="mt-2 text-sm text-textSecondary">
          在左侧目录中新建文件夹或文档，开始写作。
        </p>
      </div>
    </section>
  );
}
