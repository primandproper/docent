import type { Folder, TourSummary } from './api';

/** A folder and the tours to show under it. */
export interface FolderGroup {
  folder: Folder;
  tours: TourSummary[];
}

/**
 * Puts tours under their folders, in folder order. While searching, a folder with nothing that matches is left
 * out; otherwise an empty folder is kept, so it can still be seen and removed.
 */
export function groupByFolder(folders: Folder[], tours: TourSummary[], searching: boolean): FolderGroup[] {
  return folders
    .map((folder) => ({ folder, tours: tours.filter((tour) => tour.folder === folder.root) }))
    .filter((group) => !searching || group.tours.length > 0);
}

/**
 * Picks the tour a link or a `nextTour` means: one in the folder it was written in if there is one, else the
 * first anywhere. A tour is never confused with a namesake in another folder.
 */
export function nearest(
  tours: TourSummary[],
  from: string,
  matches: (tour: TourSummary) => boolean,
): TourSummary | undefined {
  const candidates = tours.filter((tour) => !tour.unreadable && matches(tour));
  const here = tours.find((tour) => tour.path === from)?.folder;
  return candidates.find((tour) => tour.folder === here) ?? candidates[0];
}

/** Whether a `docent:tour/` link's path names a tour: by the end of its path, so links survive a move. */
export function namedBy(tour: TourSummary, link: string): boolean {
  return tour.path === link || tour.path.endsWith(`/${link}`);
}
