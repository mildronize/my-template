import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

interface EmptyStateProps {
  icon?: LucideIcon;
  title: string;
  description?: string;
  action?: ReactNode;
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
}: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      {Icon && (
        <div className="mb-4 rounded-full bg-[var(--accent)] p-4">
          <Icon className="h-8 w-8 text-[var(--muted-foreground)]" />
        </div>
      )}
      <h3 className="mb-1 text-lg font-semibold text-[var(--sea-ink)]">
        {title}
      </h3>
      {description && (
        <p className="mb-4 max-w-sm text-sm text-[var(--sea-ink-soft)]">
          {description}
        </p>
      )}
      {action && <div>{action}</div>}
    </div>
  );
}
