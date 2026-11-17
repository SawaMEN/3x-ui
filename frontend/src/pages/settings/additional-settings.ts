export const additionalSettingsTabs = [
  'firewall',
  'core',
  'components',
  'system',
  'swap',
  'domains',
] as const;
export type AdditionalSettingsTabKey = (typeof additionalSettingsTabs)[number];

const legacyTabs: Record<string, AdditionalSettingsTabKey> = {
  core: 'core',
  swap: 'swap',
  domains: 'domains',
  certs: 'domains',
};

export function getAdditionalSettingsTab(hash: string): AdditionalSettingsTabKey | undefined {
  const slug = hash.replace(/^#/, '');
  if (Object.hasOwn(legacyTabs, slug)) return legacyTabs[slug];
  if (slug === 'advanced') return 'firewall';
  if (!slug.startsWith('advanced/')) return undefined;
  const tab = slug.slice('advanced/'.length);
  return additionalSettingsTabs.find((key) => key === tab) ?? 'firewall';
}
