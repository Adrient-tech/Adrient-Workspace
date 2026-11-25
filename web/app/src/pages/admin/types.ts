/** ServiceSetting mirrors adrientv1.ServiceSetting (proto snake_case via the gateway). */
export interface ServiceSetting {
  service_id: string;
  enabled: boolean;
  /** When non-empty, the dashboard tile opens this URL in a new tab instead of
   *  routing to the built-in adrient page. */
  external_url?: string;
}

/** ServiceSettings mirrors adrientv1.ServiceSettings.
 *  `settings` only contains services that have been explicitly set; any
 *  service id absent from the list is enabled by default (default-on). */
export interface ServiceSettings {
  org_id: string;
  settings: ServiceSetting[];
}
