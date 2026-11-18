import { describe, expect, it } from 'vitest';
import type { Folder, TourSummary } from './api';
import { groupByFolder, namedBy, nearest } from './folders';

function tour(path: string, folder: string, title = path): TourSummary {
  return { path, folder, title, unreadable: false } as TourSummary;
}

const folders = [{ root: '/a' }, { root: '/b' }, { root: '/empty' }] as Folder[];
const tours = [
  tour('/a/.tours/feature.tour', '/a', 'Feature'),
  tour('/a/.tours/overview.tour', '/a', 'Overview'),
  tour('/b/.tours/overview.tour', '/b', 'Overview'),
  tour('/b/.tours/cron.tour', '/b', 'Cron'),
];

describe('groupByFolder', () => {
  it('keeps folder order and an empty folder', () => {
    const groups = groupByFolder(folders, tours, false);
    expect(groups.map((g) => [g.folder.root, g.tours.length])).toEqual([
      ['/a', 2],
      ['/b', 2],
      ['/empty', 0],
    ]);
  });

  it('drops folders with no match while searching', () => {
    const groups = groupByFolder(folders, [tours[3]], true);
    expect(groups.map((g) => g.folder.root)).toEqual(['/b']);
  });
});

describe('nearest', () => {
  it('prefers a tour in the same folder to a namesake elsewhere', () => {
    const named = (t: TourSummary) => namedBy(t, 'overview.tour');
    expect(nearest(tours, '/a/.tours/feature.tour', named)?.path).toBe('/a/.tours/overview.tour');
    expect(nearest(tours, '/b/.tours/cron.tour', named)?.path).toBe('/b/.tours/overview.tour');
  });

  it('reaches into another folder when its own has no such tour', () => {
    expect(nearest(tours, '/a/.tours/feature.tour', (t) => namedBy(t, 'cron.tour'))?.path).toBe('/b/.tours/cron.tour');
    expect(nearest(tours, '/a/.tours/feature.tour', (t) => t.title === 'Cron')?.path).toBe('/b/.tours/cron.tour');
  });

  it('skips a tour that can not be read, and finds nothing for a stranger', () => {
    const broken = [{ ...tours[3], unreadable: true }];
    expect(nearest(broken, '', (t) => namedBy(t, 'cron.tour'))).toBeUndefined();
    expect(nearest(tours, '', (t) => namedBy(t, 'gone.tour'))).toBeUndefined();
  });

  it('matches a tour by the end of its path only at a separator', () => {
    expect(namedBy(tours[3], 'cron.tour')).toBe(true);
    expect(namedBy(tours[3], '.tours/cron.tour')).toBe(true);
    expect(namedBy(tours[3], 'on.tour')).toBe(false);
  });
});
