import type { Video } from "../types";

/** One automatic workspace tab group, named after the actor its films share. */
export interface VideoTabGroupPlan {
  /** Canonical actor name; also the key for the session's collapse state. */
  id: string;
  label: string;
  tabs: Video[];
}

export interface VideoTabPlan {
  groups: VideoTabGroupPlan[];
  /** Video tabs that do not belong to a group, in strip order. */
  ungrouped: Video[];
}

/**
 * Partitions open video tabs into automatic actor groups.
 *
 * A group only exists when at least two open films share a performer. One tab
 * can belong to only one group, so a film with several matching actors joins
 * the largest cluster; ties go to the actor whose earliest film appears first
 * (then to the name) so the same tab set always produces the same groups.
 *
 * The cast arrives as canonical names from the shared read model
 * (`Video.actors`); 演员合并 is already applied by the backend. Groups are
 * returned in strip order and keep the tabs' original order inside each group.
 */
export function planVideoTabGroups(tabs: Video[]): VideoTabPlan {
  const castByID = new Map<number, string[]>();
  for (const tab of tabs) {
    const seen = new Set<string>();
    const cast: string[] = [];
    // Be defensive about older/partial payloads: actors is an array in the API
    // contract, but a null/absent field must not break the strip.
    for (const raw of Array.isArray(tab.actors) ? tab.actors : []) {
      const name = typeof raw === "string" ? raw.trim() : "";
      if (!name || seen.has(name)) continue;
      seen.add(name);
      cast.push(name);
    }
    castByID.set(tab.id, cast);
  }

  const unassigned = new Set(tabs.map((tab) => tab.id));
  const groups: VideoTabGroupPlan[] = [];

  while (true) {
    const counts = new Map<string, number>();
    const firstAt = new Map<string, number>();
    tabs.forEach((tab, index) => {
      if (!unassigned.has(tab.id)) return;
      for (const actor of castByID.get(tab.id) ?? []) {
        counts.set(actor, (counts.get(actor) ?? 0) + 1);
        if (!firstAt.has(actor)) firstAt.set(actor, index);
      }
    });

    // Greedy largest cluster. Every member is removed before the next round,
    // so a film never appears in two groups.
    let best: string | null = null;
    for (const [actor, count] of counts) {
      if (count < 2) continue;
      if (best === null) {
        best = actor;
        continue;
      }
      const bestCount = counts.get(best) ?? 0;
      const actorFirst = firstAt.get(actor) ?? Number.MAX_SAFE_INTEGER;
      const bestFirst = firstAt.get(best) ?? Number.MAX_SAFE_INTEGER;
      if (
        count > bestCount ||
        (count === bestCount &&
          (actorFirst < bestFirst ||
            (actorFirst === bestFirst && actor.localeCompare(best) < 0)))
      ) {
        best = actor;
      }
    }
    if (best === null) break;

    const chosen = best;
    const members = tabs.filter(
      (tab) => unassigned.has(tab.id) && (castByID.get(tab.id) ?? []).includes(chosen),
    );
    // counts already guaranteed two members; the guard keeps malformed input
    // from producing an unusable single-tab group.
    if (members.length < 2) break;
    for (const member of members) unassigned.delete(member.id);
    groups.push({ id: chosen, label: chosen, tabs: members });
  }

  // Contiguous groups follow their earliest member, so a newly formed group
  // does not jump ahead of tabs that were already to its left.
  groups.sort((a, b) => tabs.indexOf(a.tabs[0]) - tabs.indexOf(b.tabs[0]));

  return {
    groups,
    ungrouped: tabs.filter((tab) => unassigned.has(tab.id)),
  };
}
