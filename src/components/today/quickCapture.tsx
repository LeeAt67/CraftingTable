import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (title: string) => Promise<unknown>;
};

export function QuickCapture({ open, onOpenChange, onCreate }: Props) {
  const [title, setTitle] = useState("");
  const [pending, setPending] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const value = title.trim();
    if (!value) return;
    setPending(true);
    try {
      await onCreate(value);
      setTitle("");
      onOpenChange(false);
    } catch {
      // The Effect already reports the error through Toast.
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>新建待办</DialogTitle>
            <DialogDescription>先记下来，稍后再安排时间。</DialogDescription>
          </DialogHeader>
          <Input
            aria-label="待办标题"
            autoFocus
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="要做什么？"
            className="mt-5"
          />
          <DialogFooter className="mt-5">
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={pending || !title.trim()}>
              {pending ? "保存中" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
