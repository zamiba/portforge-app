// A preview of the folder name a profile will get, for the moment between
// typing a name and creating it. The backend's Slugify is the truth — this
// mirrors it (NFC, lower-case, letters and digits kept, every other run one
// hyphen, none at the ends) so the preview matches what appears afterwards.
export function slugify(name) {
  return name
    .normalize('NFC')
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
}
