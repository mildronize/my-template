import { AlertCircle } from "lucide-react";
import { Button } from "~/components/ui/button";

interface ErrorStateProps {
  message?: string;
  onRetry?: () => void;
}

export function ErrorState({
  message = "Something went wrong.",
  onRetry,
}: ErrorStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <div className="mb-4 rounded-full bg-red-50 p-4 dark:bg-red-950/30">
        <AlertCircle className="h-8 w-8 text-red-500" />
      </div>
      <h3 className="mb-1 text-lg font-semibold text-[var(--sea-ink)]">
        Error
      </h3>
      <p className="mb-4 max-w-sm text-sm text-[var(--sea-ink-soft)]">
        {message}
      </p>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  );
}
