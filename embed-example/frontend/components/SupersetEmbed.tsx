import { useEffect, useRef, useState } from "react";
import { embedDashboard } from "@superset-ui/embedded-sdk";

const DASHBOARD_ID = "2c5b735e-52d0-417b-a6ee-8db9494f187d";

// 🔥 your real filter ID
const FILTER_ID = "NATIVE_FILTER-rpYb2IKm_JiYgXccnpDIl";

export default function SupersetEmbed() {
  const ref = useRef<HTMLDivElement>(null);
  const dashboardRef = useRef<any>(null);
  const [activeOnly, setActiveOnly] = useState(true);

  // ✅ APPLY FILTER
  const applyFilter = (embed: any, isActive: boolean) => {
    const statusValue = isActive ? ["ACTIVE"] : ["INACTIVE"];

    // 1️⃣ Set filter state
    embed?.postMessage?.(
      {
        type: "setDataMask",
        payload: {
          dataMask: {
            [FILTER_ID]: {
              filterState: {
                value: statusValue,
                label: statusValue.join(", "),
              },
              extraFormData: {
                filters: [
                  {
                    col: "status",
                    op: "IN",
                    val: statusValue,
                  },
                ],
              },
              ownState: {},
            },
          },
        },
      },
      "http://159.65.7.49:8088"
    );

    // 2️⃣ Apply filters (🔥 required)
    setTimeout(() => {
      embed?.postMessage?.(
        {
          type: "applyFilters",
        },
        "http://159.65.7.49:8088"
      );
    }, 200);
  };

  useEffect(() => {
    let resizeHandler: () => void = () => {};

    const load = async () => {
      const fetchGuestToken = async () => {
        
        // 🔥 MOCK JWT: This simulates the user's login token in your React app.
        // The payload inside decodes to: {"tenant":"KAJ","subtenant":"BEKASI"}
        const mockJwt = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ0ZW5hbnQiOiJBTFNBTlRPIiwic3VidGVuYW50IjoiQUxTQU5UTyJ9.";

        const res = await fetch("http://172.29.121.22:8081/superset/guest-token", {
          method: "POST",
          headers: { 
            "Content-Type": "application/json",
            "Authorization": `Bearer ${mockJwt}` // Sending credentials to backend
          },
          body: JSON.stringify({
            dashboardId: DASHBOARD_ID,
            // Removed 'tenant' from here!
          }),
        });

        const data = await res.json();
        return data.token;
      };

      const embed = await embedDashboard({
        id: DASHBOARD_ID,
        supersetDomain: "http://159.65.7.49:8088",
        mountPoint: ref.current!,
        fetchGuestToken,
      });

      dashboardRef.current = embed;

      // ✅ Apply default filter after embed is ready
      setTimeout(() => {
        applyFilter(embed, true);
      }, 600);

      // iframe styling
      const styleIframe = () => {
        if (!ref.current) return;
        const iframe = ref.current.querySelector("iframe");
        if (!iframe) return;

        const el = iframe as HTMLIFrameElement;
        el.style.width = "100%";
        el.style.height = "80vh";
        el.style.minHeight = "600px";
        el.style.border = "0";
      };

      [200, 600, 1200].forEach((d) => setTimeout(styleIframe, d));

      resizeHandler = styleIframe;
      window.addEventListener("resize", resizeHandler);
    };

    load();

    return () => {
      if (resizeHandler) {
        window.removeEventListener("resize", resizeHandler);
      }
    };
  }, []);

  // ✅ BUTTON TOGGLE
  const toggleActive = () => {
    const next = !activeOnly;
    setActiveOnly(next);

    if (dashboardRef.current) {
      applyFilter(dashboardRef.current, next);
    }
  };

  return (
    <div>
      <button
        onClick={toggleActive}
        style={{
          marginBottom: 12,
          padding: "8px 16px",
          background: activeOnly ? "#1FA8C9" : "#888",
          color: "white",
          border: "none",
          borderRadius: 6,
          cursor: "pointer",
        }}
      >
        {activeOnly ? "ACTIVE" : "INACTIVE"}
      </button>

      <div ref={ref} style={{ width: "100%", height: "80vh" }} />
    </div>
  );
}