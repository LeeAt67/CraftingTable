import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";

export default function SettingsPage() {
  return (
    <section className="mx-auto w-full max-w-3xl px-4 py-8 min-[768px]:px-8">
      <h1 className="text-2xl font-semibold text-textPrimary">设置</h1>
      <p className="mt-2 text-sm text-textSecondary">设置功能将在后续版本中提供。</p>
      <Button asChild className="mt-6">
        <Link to="/">返回今日</Link>
      </Button>
    </section>
  );
}
