/**
 * @file adsdk.js
 * @description This SDK provides helper functions for publishers to integrate with the
 * self-hosted ad server. It simplifies fetching and rendering ads, managing user identity
 * for features like frequency capping, and tracking impressions, clicks, and custom events.
 * The SDK is designed to give publishers control over ad rendering (especially for native ads)
 * and to enable advanced customization through key-value targeting and custom event tracking.
 * 
 * NEW: Advanced Custom Parameter Support for Rich Click Attribution
 * ================================================================
 * Publishers can now pass rich contextual data that flows through to advertiser landing pages
 * via macro expansion. This solves major pain points with legacy ad servers like GAM.
 * 
 * @example
 * // 1. Set global custom parameters (applied to all ad requests)
 * AdSDK.setCustomParams({
 *   user_segment: 'premium_subscriber',
 *   content_category: 'technology',
 *   page_type: 'article'
 * });
 * 
 * // 2. Auto-capture UTM parameters from page URL
 * AdSDK.setUTMFromPage(); // Captures utm_*, gclid, fbclid, etc.
 * 
 * // 3. Set placement-specific parameters (highest priority)
 * AdSDK.setAdSlotCustomParams('premium-sidebar', {
 *   placement_context: 'above_fold_premium',
 *   inventory_type: 'premium'
 * });
 * 
 * // 4. Render ads as usual - custom params are automatically included
 * AdSDK.renderAd('premium-sidebar', 'ad-container');
 * 
 * // When users click ads, custom parameters are expanded in destination URLs:
 * // Creative click_url: "https://advertiser.com/landing?segment={CUSTOM.user_segment}&source={CUSTOM.utm_source}"
 * // Becomes: "https://advertiser.com/landing?segment=premium_subscriber&source=google"
 */
(function (window) {
  // Default URL for the ad server. Can be overridden globally or per call.
  const DEFAULT_AD_SERVER_URL = "http://localhost:8787";
  // Key used for storing the user ID in localStorage.
  const STORAGE_KEY = "ad_user_id";
  // Cached API key set via AdSDK.setApiKey(). Falls back to window.AD_SERVER_API_KEY.
  let REGISTERED_API_KEY = window.AD_SERVER_API_KEY || null;
  // Cached publisher ID set via AdSDK.setPublisherId(). Falls back to window.AD_SERVER_PUBLISHER_ID.
  let REGISTERED_PUBLISHER_ID = window.AD_SERVER_PUBLISHER_ID || null;
  // Global custom parameters set via AdSDK.setCustomParams(). Applied to all ad requests.
  let GLOBAL_CUSTOM_PARAMS = {};
  // Per-slot custom parameters set via AdSDK.setAdSlotCustomParams(). Applied to specific placements.
  let PER_SLOT_CUSTOM_PARAMS = {};

  /**
   * Resolves the ad server base URL.
   * Priority:
   * 1. `override` parameter (per-call override).
   * 2. `window.AD_SERVER_URL` (global publisher-defined override).
   * 3. `DEFAULT_AD_SERVER_URL` (default).
   * This allows publishers flexible configuration for different environments (dev, staging, prod).
   * @param {string} [override] - A specific base URL to use for this call.
   * @returns {string} The resolved base URL for the ad server.
   */
  function getBaseUrl(override) {
    return override || window.AD_SERVER_URL || DEFAULT_AD_SERVER_URL;
  }

  /**
   * Gets a stable user ID from localStorage or generates a new one.
   * This helps the server with features like frequency capping and consistent user experience.
   * @returns {string} The user ID.
   */
  function getUserId() {
    let id = localStorage.getItem(STORAGE_KEY);
    if (!id) {
      id = "user_" + Math.random().toString(36).slice(2, 12);
      localStorage.setItem(STORAGE_KEY, id);
    }
    return id;
  }

  /**
   * Extracts UTM parameters and other common tracking parameters from the current page URL.
   * This is a convenience function to automatically capture campaign attribution data.
   * @returns {Object} An object containing extracted parameters.
   */
  function extractUTMFromPage() {
    const urlParams = new URLSearchParams(window.location.search);
    const utmParams = {};
    
    // Standard UTM parameters
    const utmKeys = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content'];
    utmKeys.forEach(key => {
      const value = urlParams.get(key);
      if (value) {
        utmParams[key] = value;
      }
    });
    
    // Additional common tracking parameters
    const additionalKeys = ['gclid', 'fbclid', 'msclkid', 'twclid', 'li_fat_id'];
    additionalKeys.forEach(key => {
      const value = urlParams.get(key);
      if (value) {
        utmParams[key] = value;
      }
    });
    
    // Referrer information
    if (document.referrer) {
      try {
        const referrerUrl = new URL(document.referrer);
        utmParams.referrer_domain = referrerUrl.hostname;
        // Only include full referrer if it's not the same domain (external referrer)
        if (referrerUrl.hostname !== window.location.hostname) {
          utmParams.referrer_url = document.referrer;
        }
      } catch (e) {
        // Ignore invalid referrer URLs
      }
    }
    
    return utmParams;
  }

  /**
   * Merges custom parameters from multiple sources in priority order:
   * 1. Function parameter (highest priority - per-call override)
   * 2. Per-slot custom parameters (placement-specific)
   * 3. Global custom parameters (lowest priority - applied to all requests)
   * @param {string} placementId - The placement ID to check for slot-specific params
   * @param {Object} [requestParams] - Custom parameters passed to the specific request
   * @returns {Object} Merged custom parameters object
   */
  function mergeCustomParams(placementId, requestParams = {}) {
    // Start with global params (lowest priority)
    let merged = { ...GLOBAL_CUSTOM_PARAMS };
    
    // Override with per-slot params if they exist for this placement
    if (PER_SLOT_CUSTOM_PARAMS[placementId]) {
      merged = { ...merged, ...PER_SLOT_CUSTOM_PARAMS[placementId] };
    }
    
    // Override with request-specific params (highest priority)
    merged = { ...merged, ...requestParams };
    
    return merged;
  }

  /**
   * Fetches ad data from the ad server.
   * This function is intended for publishers who want to implement custom ad rendering logic
   * or use the ad decision data in other ways (e.g., server-to-server, custom analytics).
   * It constructs and sends an OpenRTB-like request to the `/ad` endpoint.
   *
   * @param {string} placementId - The ID of the placement being requested.
   * @param {string} [baseUrl] - Optional base URL for the ad server, overriding global/default.
   * @param {Object} [keyValues] - Optional. An object of custom key-value pairs provided by the publisher
   *                               for advanced targeting (e.g., `{ "category": "sports", "page_type": "article" }`).
   *                               These are sent in `request.ext.kv`.
   * @param {string} [publisherId] - Optional publisher ID override.
   * @param {string} [apiKey] - Optional API key override.
   * @param {Object} [customParams] - Optional custom parameters for macro expansion, merged with global/slot-specific params.
   * @returns {Promise<Object>} A promise that resolves with the parsed JSON response (OpenRTBResponse) from the ad server.
   * @throws {Error} If the network request fails or the server returns an error status.
   */
  async function fetchAd(placementId, baseUrl, keyValues, publisherId, apiKey, customParams) {
    const userId = getUserId();
    // Generate a unique request ID for tracking and debugging.
    const requestId = "req_" + Math.random().toString(36).slice(2, 10);

    const resolvedPublisherId =
      publisherId || REGISTERED_PUBLISHER_ID || window.AD_SERVER_PUBLISHER_ID;
    const body = {
      id: requestId,
      imp: [{ id: "1", tagid: placementId }], // Assuming one impression per request for simplicity.
      user: { id: userId },
      device: { ua: navigator.userAgent }, // Basic device info. Server can infer IP.
      ext: { publisher_id: resolvedPublisherId },
    };

    // Attach custom key-values if provided by the publisher. This is a powerful customization feature.
    if (keyValues && Object.keys(keyValues).length > 0) {
      body.ext.kv = keyValues;
    }

    // Merge custom parameters from all sources (global, per-slot, and request-specific)
    const mergedCustomParams = mergeCustomParams(placementId, customParams);
    if (Object.keys(mergedCustomParams).length > 0) {
      body.ext.custom_params = mergedCustomParams;
    }

    const resolvedBaseUrl = getBaseUrl(baseUrl);
    // Include debug=1 only in development environments
    const isDevelopment = window.location.hostname === 'localhost' || 
                         window.location.hostname === '127.0.0.1' ||
                         window.location.hostname.includes('dev') ||
                         window.location.protocol === 'file:';
    const debugParam = isDevelopment ? '?debug=1' : '';
    const requestUrl = `${resolvedBaseUrl}/ad${debugParam}`;

    const headers = { "Content-Type": "application/json" };
    const resolvedApiKey =
      apiKey || REGISTERED_API_KEY || window.AD_SERVER_API_KEY;
    if (resolvedApiKey) {
      headers["X-API-Key"] = resolvedApiKey;
    }

    const res = await fetch(requestUrl, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
    });

    if (!res.ok) {
      throw new Error(
        `AdSDK: Ad server request to ${requestUrl} failed with HTTP status ${res.status}`,
      );
    }
    return await res.json();
  }

  /**
   * Internal helper function to send a tracking pixel request.
   * @param {string} urlPath - The path and query string of the tracking URL (e.g., /impression?t=TOKEN).
   * @param {string} [baseUrl] - Optional base URL for the ad server.
   */
  function sendPixel(urlPath, baseUrl) {
    const img = new Image();
    img.src = getBaseUrl(baseUrl) + urlPath;
  }

  /**
   * Triggers an impression tracking event by requesting the provided impression URL.
   * This should be called when the ad is considered viewable by the publisher.
   * @param {string} impressionUrl - The `impurl` from the ad response.
   * @param {string} [baseUrl] - Optional base URL for the ad server.
   */
  function sendImpression(impressionUrl, baseUrl) {
    if (!impressionUrl) return;
    sendPixel(impressionUrl, baseUrl);
  }

  /**
   * Triggers a click tracking event by requesting the provided click URL.
   * This should be called when the user clicks on the ad.
   * @param {string} clickUrl - The `clkurl` from the ad response.
   * @param {string} [baseUrl] - Optional base URL for the ad server.
   */
  function sendClick(clickUrl, baseUrl) {
    if (!clickUrl) return;
    sendPixel(clickUrl, baseUrl);
  }

  /**
   * Triggers a custom event tracking event.
   * Allows publishers to track specific interactions with an ad beyond impressions and clicks.
   * @param {string} [baseUrl] - Optional base URL for the ad server.
   * @param {string} eventTrackingUrl - The `evturl` from the ad response.
   * @param {string} eventType - The publisher-defined type of the event (e.g., "like", "video_view").
   */
  function sendEvent(baseUrl, eventTrackingUrl, eventType) {
    if (!eventTrackingUrl || !eventType) return;
    sendPixel(
      `${eventTrackingUrl}&type=${encodeURIComponent(eventType)}`,
      baseUrl,
    );
  }

  /**
   * Submit an ad report to the server.
   * @param {string} reportUrl - The `repturl` from the ad response.
  * @param {string} reason - Report reason code.
  * @param {string} [baseUrl]
  */
  function reportAd(reportUrl, reason, baseUrl) {
    if (!reportUrl || !reason)
      return Promise.reject("missing report url or reason");
    const token = reportUrl.split("t=")[1];
    return fetch(getBaseUrl(baseUrl) + "/report", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        token: token,
        reason: reason,
      }),
    });
  }

  /**
   * Fetches and renders a standard HTML ad into a specified container.
   * This function simplifies displaying HTML ads by handling iframe creation and basic event tracking.
   *
   * @param {string} placementId - The ID of the placement for which to request an ad.
   * @param {string} containerId - The ID of the DOM element where the ad should be rendered.
   * @param {string} [baseUrl] - Optional base URL for the ad server.
   * @param {Object} [keyValues] - Optional custom key-value pairs for targeting.
   * @param {string} [publisherId] - Optional publisher ID override.
   * @param {string} [apiKey] - Optional API key override.
   * @param {Object} [customParams] - Optional custom parameters for macro expansion, merged with global/slot-specific params.
   */
  function renderAd(
    placementId,
    containerId,
    baseUrl,
    keyValues,
    publisherId,
    apiKey,
    customParams,
  ) {
    const container = document.getElementById(containerId);
    if (!container) {
      console.warn("AdSDK: container not found:", containerId);
      return;
    }

    fetchAd(placementId, baseUrl, keyValues, publisherId, apiKey, customParams)
      .then((ortbRes) => {
        const seatbid = ortbRes.seatbid || [];
        if (seatbid.length === 0 || (seatbid[0].bid || []).length === 0) {
          // No ad available (no-bid scenario).
          console.info(
            `AdSDK: No ad available for placement '${placementId}'. Reason: ${ortbRes.nbr || "Unknown"}.`,
          );
          container.innerHTML = ""; // Clear the container or publisher can show fallback content.
          // Dispatch a custom event that publishers can listen to for handling no-bid scenarios (e.g., collapse ad slot).
          const noAdEvent = new CustomEvent("AdSDK:noAd", {
            detail: { placementId: placementId, reason: ortbRes.nbr },
          });
          container.dispatchEvent(noAdEvent);
          // Also dispatch on window for global listeners if needed, though container specific is often better.
          // window.dispatchEvent(noAdEvent);
          return;
        }

        const bid = seatbid[0].bid[0]; // Assuming one bid.
        const { adm, impurl, clkurl, evturl, repturl } = bid;

        // Generate a unique ID for this ad instance to correlate postMessage events.
        const adInstanceId =
          "adInst_" + Math.random().toString(36).slice(2, 10);

        // Listener for messages from the sandboxed iframe (for clicks and custom events).
        // This enables communication from the isolated creative back to the SDK on the parent page.
        function handleAdMessage(event) {
          // Basic security: check origin if possible, though '*' is used here for simplicity in srcdoc.
          // if (event.origin !== getBaseUrl(baseUrl)) return;
          if (event.data && event.data.adInstanceId === adInstanceId) {
            if (event.data.type === "ad_sdk_click") {
              // For banner ads, navigate to the tracking URL which will handle click tracking and redirect
              if (clkurl) {
                window.open(getBaseUrl(baseUrl) + clkurl, '_blank');
              }
            } else if (
              event.data.type === "ad_sdk_event" &&
              event.data.eventType
            ) {
              if (evturl) {
                // Check if event tracking URL is available
                sendEvent(baseUrl, evturl, event.data.eventType);
              } else {
                console.warn(
                  `AdSDK: Event tracking URL not available for event '${event.data.eventType}'.`,
                );
              }
            }
          }
        }
        window.addEventListener("message", handleAdMessage);

        // store report URL on the container for convenience
        if (repturl) {
          container.dataset.reportUrl = repturl;
        }

        // Create wrapper div to hold iframe and report link
        const wrapper = document.createElement("div");
        wrapper.style.position = "relative";
        wrapper.style.width = "100%";
        wrapper.style.height = "100%";

        // Create and configure a sandboxed iframe to render the HTML ad.
        // The sandbox attribute enhances security for the publisher's page by restricting iframe capabilities.
        const iframe = document.createElement("iframe");
        iframe.style.border = "0"; // Common styling.
        iframe.style.width = "100%"; // Default to container width.
        iframe.style.height = "100%"; // Default to container height.
        // 'allow-scripts' is needed for ad functionality. 'allow-popups' if ads might open new tabs (use with caution).
        // 'allow-same-origin' is NOT set, so it's treated as cross-origin, which is safer.
        iframe.setAttribute("sandbox", "allow-scripts allow-popups");

        // Create report link overlay with meatball menu style
        const reportLink = document.createElement("button");
        reportLink.innerHTML = "⋯"; // Three dots (meatball menu)
        reportLink.style.position = "absolute";
        reportLink.style.top = "4px";
        reportLink.style.right = "4px";
        reportLink.style.zIndex = "1000";
        reportLink.style.background = "rgba(0, 0, 0, 0.6)";
        reportLink.style.color = "white";
        reportLink.style.border = "none";
        reportLink.style.borderRadius = "50%";
        reportLink.style.width = "20px";
        reportLink.style.height = "20px";
        reportLink.style.fontSize = "12px";
        reportLink.style.cursor = "pointer";
        reportLink.style.fontFamily = "Arial, sans-serif";
        reportLink.style.display = "flex";
        reportLink.style.alignItems = "center";
        reportLink.style.justifyContent = "center";
        reportLink.style.lineHeight = "1";
        reportLink.style.transition = "background 0.2s ease";
        reportLink.title = "Report this ad";
        
        // Hover effect
        reportLink.addEventListener("mouseenter", () => {
          reportLink.style.background = "rgba(0, 0, 0, 0.8)";
        });
        reportLink.addEventListener("mouseleave", () => {
          reportLink.style.background = "rgba(0, 0, 0, 0.6)";
        });
        
        reportLink.addEventListener("click", () => {
          if (repturl) {
            showReportModal(repturl, baseUrl);
          }
        });

        // Add iframe and report link to wrapper
        wrapper.appendChild(iframe);
        wrapper.appendChild(reportLink);

        // Clear container and append wrapper
        container.innerHTML = "";
        container.appendChild(wrapper);

        // Construct the iframe content. This includes the ad markup (adm) and a small script
        // that allows the creative to send 'click' and custom 'event' messages to the parent window (this SDK).
        // The `AdSDKInternal_Event` function is exposed within the iframe for creatives to call.
        const iframeMarkup = `<!DOCTYPE html>
          <html lang="en">
          <head><meta charset="UTF-F"><title>Ad</title><style>body{margin:0;padding:0;display:flex;align-items:center;justify-content:center;}</style></head>
          <body>
            ${adm}
            <script type="text/javascript">
              // Function exposed inside the iframe for the creative to call for custom events.
              window.AdSDK_Creative_Event = function(eventType) {
                parent.postMessage({ type: 'ad_sdk_event', adInstanceId: '${adInstanceId}', eventType: eventType }, '*');
              };
              // Global click listener within the iframe to report clicks.
              document.body.addEventListener('click', function() {
                parent.postMessage({ type: 'ad_sdk_click', adInstanceId: '${adInstanceId}' }, '*');
              });
            <\/script>
          </body></html>`;
        iframe.srcdoc = iframeMarkup; // srcdoc is widely supported and good for self-contained content.

        // Trigger the impression tracking pixel once the ad is rendered.
        sendImpression(impurl, baseUrl);

        // TODO: Consider adding cleanup for the window.removeEventListener when the ad is removed or replaced.
      })
      .catch((err) => {
        console.warn(
          `AdSDK: Error fetching or rendering ad for placement '${placementId}':`,
          err,
        );
        // Optionally dispatch a specific error event on the container.
        const errorEvent = new CustomEvent("AdSDK:error", {
          detail: { placementId: placementId, error: err.message },
        });
        container.dispatchEvent(errorEvent);
      });
  }

  /**
   * Fetches ad data and renders a native ad using a publisher-provided template function.
   * This gives publishers full control over the look and feel of native ads, ensuring seamless
   * integration with their site's design.
   *
   * @param {string} placementId - The ID of the native placement.
   * @param {string} containerId - The ID of the DOM element where the native ad should be rendered.
   * @param {(assets:Object) => string} template - A function provided by the publisher that takes
   *                                               a JSON object of native ad assets (from `bid.adm`)
   *                                               and returns an HTML string to be rendered.
   *                                               The structure of `assets` is defined by the publisher
   *                                               during creative setup on the ad server.
   * @param {string} [baseUrl] - Optional base URL for the ad server.
   * @param {Object} [keyValues] - Optional custom key-value pairs for targeting.
   * @param {string} [publisherId] - Optional publisher ID override.
   * @param {string} [apiKey] - Optional API key override.
   * @param {Object} [customParams] - Optional custom parameters for macro expansion, merged with global/slot-specific params.
   */
  function renderNativeAd(
    placementId,
    containerId,
    template,
    baseUrl,
    keyValues,
    publisherId,
    apiKey,
    customParams,
  ) {
    const container = document.getElementById(containerId);
    if (!container) {
      console.warn("AdSDK: container not found:", containerId);
      return;
    }

    fetchAd(placementId, baseUrl, keyValues, publisherId, apiKey, customParams)
      .then((ortbRes) => {
        const bid = ortbRes.seatbid?.[0]?.bid?.[0];
        if (!bid) {
          console.info(
            `AdSDK: No native ad available for placement '${placementId}'. Reason: ${ortbRes.nbr || "Unknown"}.`,
          );
          container.innerHTML = "";
          // Dispatch no-ad event for publisher handling.
          const noAdEvent = new CustomEvent("AdSDK:noAd", {
            detail: { placementId: placementId, reason: ortbRes.nbr },
          });
          container.dispatchEvent(noAdEvent);
          return;
        }

        // Native ad assets are expected to be a JSON string in bid.adm.
        // Publishers define this JSON structure when setting up the native creative on the server.
        let assets;
        try {
          assets = typeof bid.adm === "string" ? JSON.parse(bid.adm) : bid.adm;
        } catch (e) {
          console.error(
            "AdSDK: Failed to parse native ad assets (bid.adm). Ensure it's valid JSON.",
            e,
          );
          // Dispatch error event
          const errorEvent = new CustomEvent("AdSDK:error", {
            detail: {
              placementId: placementId,
              error: "Failed to parse native assets.",
            },
          });
          container.dispatchEvent(errorEvent);
          return;
        }

        const { impurl, clkurl, evturl, repturl } = bid;

        // Add tracking URLs to assets for template access
        // Note: clkurl, impurl, evturl are already complete paths (e.g., "/click?t=...")
        const assetsWithTracking = {
          ...assets,
          clickUrl: clkurl || null,
          impressionUrl: impurl || null,
          eventUrl: evturl || null
        };

        // The publisher-provided template function is responsible for creating the HTML from the assets.
        // This gives complete rendering control to the publisher.
        const html =
          typeof template === "function"
            ? template(assetsWithTracking) // Publisher's custom templating logic.
            : `<!-- Default native rendering if no template provided -->
             <div>
               <h2>${assetsWithTracking.title || "Native Ad"}</h2>
               ${assetsWithTracking.image ? `<img src="${assetsWithTracking.image.url}" alt="${assetsWithTracking.image.alt || assetsWithTracking.title || ""}" width="${assetsWithTracking.image.w || ""}" height="${assetsWithTracking.image.h || ""}"/>` : ""}
               <p>${assetsWithTracking.description || ""}</p>
               ${assetsWithTracking.clickUrl ? `<a href="${assetsWithTracking.clickUrl}" target="_blank">Learn More</a>` : ""}
             </div>`;
        container.innerHTML = html;
        sendImpression(impurl, baseUrl); // Track impression once rendered.

        if (repturl) {
          container.dataset.reportUrl = repturl;
          
          // Auto-append report link if not already present in template and not disabled
          if (!container.querySelector('[data-adsdk-report]') && !container.hasAttribute('data-disable-reporting')) {
            const reportLink = document.createElement("button");
            reportLink.innerHTML = "⋯"; // Three dots (meatball menu)
            reportLink.style.fontSize = "12px";
            reportLink.style.width = "20px";
            reportLink.style.height = "20px";
            reportLink.style.marginTop = "5px";
            reportLink.style.background = "#f8f9fa";
            reportLink.style.border = "1px solid #dee2e6";
            reportLink.style.borderRadius = "50%";
            reportLink.style.cursor = "pointer";
            reportLink.style.color = "#6c757d";
            reportLink.style.display = "flex";
            reportLink.style.alignItems = "center";
            reportLink.style.justifyContent = "center";
            reportLink.style.lineHeight = "1";
            reportLink.style.transition = "background 0.2s ease";
            reportLink.title = "Report this ad";
            reportLink.setAttribute('data-adsdk-report', 'true');
            
            // Hover effect
            reportLink.addEventListener("mouseenter", () => {
              reportLink.style.background = "#e9ecef";
            });
            reportLink.addEventListener("mouseleave", () => {
              reportLink.style.background = "#f8f9fa";
            });
            
            reportLink.addEventListener("click", () => {
              showReportModal(repturl, baseUrl);
            });
            container.appendChild(reportLink);
          }
        }

        // Event listener for clicks on the native ad container.
        // It handles both general clicks (tracked via clkurl) and specific custom event clicks
        // (elements with `data-ad-event` attribute, tracked via evturl).
        // Note: Since we now provide proper tracking URLs in the template, most clicks will be handled
        // by the links themselves. This listener serves as a fallback and for custom events.
        container.addEventListener("click", (event) => {
          const targetElement = event.target.closest("[data-ad-event]");
          if (targetElement && evturl) {
            // If a clickable element within the native ad has `data-ad-event="eventType"`,
            // track that custom event. This allows publishers to define multiple interaction points.
            const eventType = targetElement.getAttribute("data-ad-event");
            if (eventType) {
              sendEvent(baseUrl, evturl, eventType);
              // Optionally, prevent default if it's a link that shouldn't navigate immediately
              // or if the click is fully handled by the event.
              // event.preventDefault();
              return; // Custom event handled, don't process as a general click.
            }
          }
          // Only track clicks that aren't already handled by links with proper tracking URLs
          // This prevents double-tracking when users click on tracking links
          if (!event.target.closest('a[href*="/click?"]')) {
            sendClick(clkurl, baseUrl);
          }
        });
      })
      .catch((err) => {
        console.warn(
          `AdSDK: Error fetching or rendering native ad for placement '${placementId}':`,
          err,
        );
        const errorEvent = new CustomEvent("AdSDK:error", {
          detail: { placementId: placementId, error: err.message },
        });
        container.dispatchEvent(errorEvent);
      });
  }

  /**
   * Registers a publisher API key for subsequent requests.
   * Calling this once allows renderAd/renderNativeAd/fetchAd to omit the apiKey parameter.
   * @param {string} key - The publisher API key.
   */
  function setApiKey(key) {
    REGISTERED_API_KEY = key;
  }

  /**
   * Registers a publisher ID for subsequent requests.
   * Calling this once allows renderAd/renderNativeAd/fetchAd to omit the publisherId parameter.
   * @param {number} id - The publisher ID.
   */
  function setPublisherId(id) {
    REGISTERED_PUBLISHER_ID = id;
  }

  /**
   * Sets global custom parameters that will be applied to all ad requests.
   * These parameters are available for macro expansion in advertiser destination URLs.
   * Can be called multiple times - parameters are merged (newer calls override existing keys).
   * 
   * @param {Object} params - Object containing custom parameter key-value pairs
   * @example
   * // Set global user segment and content context for all ads
   * AdSDK.setCustomParams({
   *   user_segment: 'premium_subscriber',
   *   content_category: 'technology',
   *   page_type: 'article'
   * });
   */
  function setCustomParams(params) {
    if (params && typeof params === 'object') {
      GLOBAL_CUSTOM_PARAMS = { ...GLOBAL_CUSTOM_PARAMS, ...params };
    }
  }

  /**
   * Auto-extracts UTM parameters and common tracking parameters from the current page URL
   * and sets them as global custom parameters. This is a convenience method for capturing
   * campaign attribution data automatically.
   * 
   * @example
   * // Automatically capture UTM parameters from page URL
   * // If current URL is: /page?utm_source=google&utm_campaign=summer&gclid=abc123
   * AdSDK.setUTMFromPage();
   * // This sets global custom params: { utm_source: 'google', utm_campaign: 'summer', gclid: 'abc123' }
   */
  function setUTMFromPage() {
    const utmParams = extractUTMFromPage();
    if (Object.keys(utmParams).length > 0) {
      setCustomParams(utmParams);
    }
  }

  /**
   * Sets custom parameters for a specific ad placement/slot.
   * These parameters are applied only to ads for the specified placement ID.
   * Per-slot parameters take priority over global parameters.
   * 
   * @param {string} placementId - The placement ID to apply parameters to
   * @param {Object} params - Object containing custom parameter key-value pairs
   * @example
   * // Set placement-specific context for premium sidebar ads
   * AdSDK.setAdSlotCustomParams('premium-sidebar', {
   *   placement_context: 'above_fold_premium',
   *   inventory_type: 'premium'
   * });
   */
  function setAdSlotCustomParams(placementId, params) {
    if (placementId && params && typeof params === 'object') {
      if (!PER_SLOT_CUSTOM_PARAMS[placementId]) {
        PER_SLOT_CUSTOM_PARAMS[placementId] = {};
      }
      PER_SLOT_CUSTOM_PARAMS[placementId] = { ...PER_SLOT_CUSTOM_PARAMS[placementId], ...params };
    }
  }

  /**
   * Gets the current global custom parameters.
   * Useful for debugging or inspecting what parameters are currently set.
   * 
   * @returns {Object} Copy of current global custom parameters
   */
  function getCustomParams() {
    return { ...GLOBAL_CUSTOM_PARAMS };
  }

  /**
   * Gets custom parameters for a specific ad slot.
   * 
   * @param {string} placementId - The placement ID to get parameters for
   * @returns {Object} Copy of custom parameters for the specified slot
   */
  function getAdSlotCustomParams(placementId) {
    return PER_SLOT_CUSTOM_PARAMS[placementId] ? { ...PER_SLOT_CUSTOM_PARAMS[placementId] } : {};
  }

  /**
   * Clears all global custom parameters.
   */
  function clearCustomParams() {
    GLOBAL_CUSTOM_PARAMS = {};
  }

  /**
   * Clears custom parameters for a specific ad slot.
   * 
   * @param {string} placementId - The placement ID to clear parameters for
   */
  function clearAdSlotCustomParams(placementId) {
    if (PER_SLOT_CUSTOM_PARAMS[placementId]) {
      delete PER_SLOT_CUSTOM_PARAMS[placementId];
    }
  }

  /**
   * Valid report reasons as defined by the ad server
   */
  const REPORT_REASONS = [
    { code: 'offensive', display: 'Offensive or inappropriate', description: 'Contains offensive or inappropriate content' },
    { code: 'malware', display: 'Malware or security risk', description: 'Links to malware or attempts phishing' },
    { code: 'misleading', display: 'Misleading or scam', description: 'Deceptive or fraudulent ad' },
    { code: 'irrelevant', display: 'Irrelevant', description: 'Not relevant to the page content' },
    { code: 'other', display: 'Other', description: 'Other issue' }
  ];

  /**
   * Shows a dropdown menu for selecting a report reason and submits it.
   * @param {string} reportUrl - The `repturl` from the bid response.
   * @param {string} [baseUrl]
   */
  function showReportModal(reportUrl, baseUrl) {
    // Remove any existing modal
    const existingModal = document.getElementById('adsdk-report-modal');
    if (existingModal) {
      existingModal.remove();
    }

    // Create modal backdrop
    const backdrop = document.createElement('div');
    backdrop.id = 'adsdk-report-modal';
    backdrop.style.cssText = `
      position: fixed;
      top: 0;
      left: 0;
      width: 100%;
      height: 100%;
      background: rgba(0, 0, 0, 0.5);
      z-index: 10000;
      display: flex;
      align-items: center;
      justify-content: center;
      font-family: Arial, sans-serif;
    `;

    // Create modal content
    const modal = document.createElement('div');
    modal.style.cssText = `
      background: white;
      padding: 20px;
      border-radius: 8px;
      max-width: 400px;
      width: 90%;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
    `;

    // Create modal header
    const header = document.createElement('div');
    header.style.cssText = `
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 15px;
    `;
    
    const title = document.createElement('h3');
    title.textContent = 'Report Ad';
    title.style.cssText = `
      margin: 0;
      color: #333;
      font-size: 18px;
    `;

    const closeBtn = document.createElement('button');
    closeBtn.textContent = '×';
    closeBtn.style.cssText = `
      background: none;
      border: none;
      font-size: 24px;
      cursor: pointer;
      color: #666;
      padding: 0;
      width: 30px;
      height: 30px;
      display: flex;
      align-items: center;
      justify-content: center;
    `;
    closeBtn.addEventListener('click', () => backdrop.remove());

    header.appendChild(title);
    header.appendChild(closeBtn);

    // Create reason selection
    const reasonLabel = document.createElement('label');
    reasonLabel.textContent = 'Why are you reporting this ad?';
    reasonLabel.style.cssText = `
      display: block;
      margin-bottom: 8px;
      font-weight: bold;
      color: #333;
    `;

    const reasonSelect = document.createElement('select');
    reasonSelect.style.cssText = `
      width: 100%;
      padding: 8px;
      margin-bottom: 15px;
      border: 1px solid #ddd;
      border-radius: 4px;
      font-size: 14px;
    `;

    // Add default option
    const defaultOption = document.createElement('option');
    defaultOption.value = '';
    defaultOption.textContent = 'Select a reason...';
    reasonSelect.appendChild(defaultOption);

    // Add report reason options
    REPORT_REASONS.forEach(reason => {
      const option = document.createElement('option');
      option.value = reason.code;
      option.textContent = reason.display;
      option.title = reason.description;
      reasonSelect.appendChild(option);
    });

    // Create action buttons
    const buttonContainer = document.createElement('div');
    buttonContainer.style.cssText = `
      display: flex;
      gap: 10px;
      justify-content: flex-end;
    `;

    const cancelBtn = document.createElement('button');
    cancelBtn.textContent = 'Cancel';
    cancelBtn.style.cssText = `
      padding: 8px 16px;
      border: 1px solid #ddd;
      background: white;
      color: #333;
      border-radius: 4px;
      cursor: pointer;
      font-size: 14px;
    `;
    cancelBtn.addEventListener('click', () => backdrop.remove());

    const submitBtn = document.createElement('button');
    submitBtn.textContent = 'Report';
    submitBtn.style.cssText = `
      padding: 8px 16px;
      border: none;
      background: #dc3545;
      color: white;
      border-radius: 4px;
      cursor: pointer;
      font-size: 14px;
    `;

    submitBtn.addEventListener('click', () => {
      const selectedReason = reasonSelect.value;
      if (!selectedReason) {
        alert('Please select a reason for reporting this ad.');
        return;
      }

      submitBtn.disabled = true;
      submitBtn.textContent = 'Reporting...';
      
      reportAd(reportUrl, selectedReason, baseUrl)
        .then(() => {
          backdrop.remove();
          // Show success message
          const successMsg = document.createElement('div');
          successMsg.textContent = 'Ad reported successfully. Thank you for your feedback.';
          successMsg.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: #28a745;
            color: white;
            padding: 10px 15px;
            border-radius: 4px;
            z-index: 10001;
            font-family: Arial, sans-serif;
            font-size: 14px;
          `;
          document.body.appendChild(successMsg);
          setTimeout(() => successMsg.remove(), 3000);
        })
        .catch((err) => {
          console.warn("AdSDK: report failed", err);
          submitBtn.disabled = false;
          submitBtn.textContent = 'Report';
          alert('Failed to report ad. Please try again.');
        });
    });

    buttonContainer.appendChild(cancelBtn);
    buttonContainer.appendChild(submitBtn);

    // Assemble modal
    modal.appendChild(header);
    modal.appendChild(reasonLabel);
    modal.appendChild(reasonSelect);
    modal.appendChild(buttonContainer);
    backdrop.appendChild(modal);

    // Close on backdrop click
    backdrop.addEventListener('click', (e) => {
      if (e.target === backdrop) {
        backdrop.remove();
      }
    });

    // Close on escape key
    const escapeHandler = (e) => {
      if (e.key === 'Escape') {
        backdrop.remove();
        document.removeEventListener('keydown', escapeHandler);
      }
    };
    document.addEventListener('keydown', escapeHandler);

    document.body.appendChild(backdrop);
    reasonSelect.focus();
  }

  /**
   * Helper function for publishers to generate report link HTML for native ad templates.
   * This allows publishers to customize the report link styling and placement.
   * @param {string} [text] - Custom text for the report link (default: "Report Ad")
   * @param {string} [className] - CSS class name for custom styling
   * @param {Object} [style] - Inline style object for the report link
   * @returns {string} HTML string for the report link
   */
  function getReportLinkHTML(text = "Report Ad", className = "", style = {}) {
    const defaultStyle = {
      fontSize: "10px",
      padding: "2px 6px",
      background: "#f8f9fa",
      border: "1px solid #dee2e6",
      borderRadius: "3px",
      cursor: "pointer",
      color: "#6c757d",
      textDecoration: "none",
      display: "inline-block",
      ...style
    };
    
    const styleString = Object.entries(defaultStyle)
      .map(([key, value]) => `${key.replace(/([A-Z])/g, '-$1').toLowerCase()}: ${value}`)
      .join('; ');
    
    return `<button data-adsdk-report="true" class="${className}" style="${styleString}" onclick="AdSDK.showReportModal(this.closest('[data-report-url]').dataset.reportUrl || this.closest('.native-ad, .ad-placement').dataset.reportUrl)" title="Report this ad">⋯</button>`;
  }

  // Expose the public API functions to the window object, under the AdSDK namespace.
  // This allows publishers to call AdSDK.renderAd(), AdSDK.fetchAd(), etc.
  window.AdSDK = {
    renderAd,
    fetchAd,
    renderNativeAd,
    setApiKey,
    setPublisherId,
    // Custom parameters API
    setCustomParams,
    setUTMFromPage,
    setAdSlotCustomParams,
    getCustomParams,
    getAdSlotCustomParams,
    clearCustomParams,
    clearAdSlotCustomParams,
    // Reporting API
    reportAd,
    showReportModal,
    getReportLinkHTML,
  };
})(window);
