import type { HTMLAttributes } from "react"
import { cn } from "@/components/utils"

function Skeleton({
  className,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("animate-pulse rounded-md bg-surfaceSubtle", className)}
      {...props}
    />
  )
}

export { Skeleton }
