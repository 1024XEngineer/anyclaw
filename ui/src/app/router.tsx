import { createHashRouter } from "react-router-dom";

import { AppShell } from "@/layouts/AppShell/AppShell";
import { ChannelsPage } from "@/pages/Channels/ChannelsPage";
import { ChatHomePage } from "@/pages/ChatHome/ChatHomePage";
import { DiscoveryPage } from "@/pages/Discovery/DiscoveryPage";
import { MarketPage } from "@/pages/Market/MarketPage";
import { StudioPage } from "@/pages/Studio/StudioPage";

export const router = createHashRouter([
  {
    element: <AppShell />,
    path: "/",
    children: [
      {
        element: <ChatHomePage />,
        index: true,
      },
      {
        element: <MarketPage />,
        path: "market",
      },
      {
        element: <ChannelsPage />,
        path: "channels",
      },
      {
        element: <DiscoveryPage />,
        path: "discovery",
      },
      {
        element: <StudioPage />,
        path: "studio",
      },
    ],
  },
]);
