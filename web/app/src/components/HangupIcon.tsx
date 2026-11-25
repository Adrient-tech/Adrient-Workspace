import { SvgIcon, type SvgIconProps } from "@mui/joy";

/**
 * Hang-up glyph: a handset lying flat with its ear and mouth pieces pointing
 * down, like a phone being put back on its cradle. Inline SVG, inherits the
 * surrounding text colour (white on the red call buttons).
 */
export function HangupIcon(props: SvgIconProps) {
  return (
    <SvgIcon viewBox="0 0 24 24" {...props}>
      <path
        fill="currentColor"
        d="M12 8.2c-3.9 0-7.3 1.3-9.4 3.5-.5.6-.7 1.2-.7 1.9v1.4c0 .9.6 1.6 1.5 1.8l2.5.5c.9.2 1.8-.3 2.1-1.2l.4-1.3c.1-.4.4-.7.8-.8 1.5-.4 3.1-.4 4.6 0 .4.1.7.4.8.8l.4 1.3c.3.9 1.2 1.4 2.1 1.2l2.5-.5c.9-.2 1.5-.9 1.5-1.8v-1.4c0-.7-.2-1.3-.7-1.9C19.3 9.5 15.9 8.2 12 8.2z"
      />
    </SvgIcon>
  );
}
