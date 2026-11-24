/**
 * The name a scrape provider is shown under on both clients.
 *
 * The record stores the provider's id ("javbus", "avmoo", "fc2cmadb"), which is
 * a transport detail, not a label: the detail panel names where the metadata came
 * from, and that name follows the provider that actually answered — a fallback
 * to Avmoo must not keep saying JavBus. An unknown id is shown as stored, so a
 * new provider is visible instead of silently labelled as another one.
 */
const PROVIDER_NAMES: Record<string, string> = {
  javbus: "JavBus",
  avmoo: "Avmoo",
  fc2cmadb: "FC2CMADB",
};

export function providerDisplayName(provider?: string | null): string {
  const id = provider?.trim() ?? "";
  if (!id) return "";
  return PROVIDER_NAMES[id.toLowerCase()] ?? id;
}
