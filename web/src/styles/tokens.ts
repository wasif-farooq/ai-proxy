/**
 * Design Tokens — inspired by the Airbnb design system (DESIGN.md)
 *
 * Colors, typography, spacing, border radii, and shadows are defined
 * here so they can be imported directly by components.  Tailwind CSS
 * mirrors these values in its theme (index.css) for utility classes.
 */

/* ─── Colors ─────────────────────────────────────────────── */

export const colors = {
  primary: '#ff385c',
  primaryActive: '#e00b41',
  primaryDisabled: '#ffd1da',
  primaryErrorText: '#c13515',
  primaryErrorTextHover: '#b32505',
  luxe: '#460479',
  plus: '#92174d',
  ink: '#222222',
  body: '#3f3f3f',
  muted: '#6a6a6a',
  mutedSoft: '#929292',
  hairline: '#dddddd',
  hairlineSoft: '#ebebeb',
  borderStrong: '#c1c1c1',
  canvas: '#ffffff',
  surfaceSoft: '#f7f7f7',
  surfaceCard: '#ffffff',
  surfaceStrong: '#f2f2f2',
  onPrimary: '#ffffff',
  onDark: '#ffffff',
  legalLink: '#428bff',
  starRating: '#222222',
  scrim: '#000000',
} as const;

export type ColorKey = keyof typeof colors;

/* ─── Typography ─────────────────────────────────────────── */

export const typography = {
  ratingDisplay: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 64,
    fontWeight: 700,
    lineHeight: 1.1,
    letterSpacing: '-1px',
  },
  displayXl: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 28,
    fontWeight: 700,
    lineHeight: 1.43,
    letterSpacing: 0,
  },
  displayLg: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 22,
    fontWeight: 500,
    lineHeight: 1.18,
    letterSpacing: '-0.44px',
  },
  displayMd: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 21,
    fontWeight: 700,
    lineHeight: 1.43,
    letterSpacing: 0,
  },
  displaySm: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 20,
    fontWeight: 600,
    lineHeight: 1.2,
    letterSpacing: '-0.18px',
  },
  titleMd: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 16,
    fontWeight: 600,
    lineHeight: 1.25,
    letterSpacing: 0,
  },
  titleSm: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 16,
    fontWeight: 500,
    lineHeight: 1.25,
    letterSpacing: 0,
  },
  bodyMd: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 16,
    fontWeight: 400,
    lineHeight: 1.5,
    letterSpacing: 0,
  },
  bodySm: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 14,
    fontWeight: 400,
    lineHeight: 1.43,
    letterSpacing: 0,
  },
  caption: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 14,
    fontWeight: 500,
    lineHeight: 1.29,
    letterSpacing: 0,
  },
  captionSm: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 13,
    fontWeight: 400,
    lineHeight: 1.23,
    letterSpacing: 0,
  },
  badge: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 11,
    fontWeight: 600,
    lineHeight: 1.18,
    letterSpacing: 0,
  },
  microLabel: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 12,
    fontWeight: 700,
    lineHeight: 1.33,
    letterSpacing: 0,
  },
  uppercaseTag: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 8,
    fontWeight: 700,
    lineHeight: 1.25,
    letterSpacing: '0.32px',
    textTransform: 'uppercase' as const,
  },
  buttonMd: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 16,
    fontWeight: 500,
    lineHeight: 1.25,
    letterSpacing: 0,
  },
  buttonSm: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 14,
    fontWeight: 500,
    lineHeight: 1.29,
    letterSpacing: 0,
  },
  navLink: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 16,
    fontWeight: 600,
    lineHeight: 1.25,
    letterSpacing: 0,
  },
  link: {
    fontFamily: "'Inter', system-ui, sans-serif",
    fontSize: 14,
    fontWeight: 400,
    lineHeight: 1.43,
    letterSpacing: 0,
  },
} as const;

export type TypographyKey = keyof typeof typography;

/* ─── Spacing ────────────────────────────────────────────── */

export const spacing = {
  xxs: 2,
  xs: 4,
  sm: 8,
  md: 12,
  base: 16,
  lg: 24,
  xl: 32,
  xxl: 48,
  section: 64,
} as const;

export type SpacingKey = keyof typeof spacing;

/* ─── Border Radius ──────────────────────────────────────── */

export const borderRadius = {
  none: 0,
  xs: 4,
  sm: 8,
  md: 14,
  lg: 20,
  xl: 32,
  full: 9999,
} as const;

export type RadiusKey = keyof typeof borderRadius;

/* ─── Elevation ──────────────────────────────────────────── */

export const shadow = {
  card: 'rgba(0, 0, 0, 0.02) 0 0 0 1px, rgba(0, 0, 0, 0.04) 0 2px 6px, rgba(0, 0, 0, 0.1) 0 4px 8px',
  modal: 'rgba(0, 0, 0, 0.02) 0 0 0 1px, rgba(0, 0, 0, 0.16) 0 6px 16px',
} as const;
