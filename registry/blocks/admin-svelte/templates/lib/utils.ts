import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

// cn merges Tailwind classes, the shadcn-svelte convention.
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
