import type { ConfigObject } from './simple-settings';
export type OptionDescriptor = {
  kind: string;
  required?: boolean;
  enum?: (string | number)[];
  min?: number;
  max?: number;
  integer?: boolean;
  ref?: string;
  item?: OptionDescriptor;
  when?: Record<string, (string | number)[]>;
};
export type OptionSchema = Record<string, OptionDescriptor>;
export function optionVisible(descriptor: OptionDescriptor, value: ConfigObject): boolean {
  return (
    !descriptor.when ||
    Object.entries(descriptor.when).every(([key, allowed]) =>
      allowed.includes((value[key] ?? 0) as string | number),
    )
  );
}
export function patchOption(
  value: ConfigObject,
  key: string,
  next: unknown,
  schema: OptionSchema,
): ConfigObject {
  const result = { ...value };
  if (next === '' || next == null) delete result[key];
  else result[key] = next;
  // Remove only fields tied to a discriminator the user actually changed.
  // Unrelated imported/unknown options remain untouched.
  for (const [field, descriptor] of Object.entries(schema)) {
    if (descriptor.when?.[key] && !optionVisible(descriptor, result)) delete result[field];
  }
  return result;
}
