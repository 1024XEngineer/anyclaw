import { createHashRouter } from "react-router-dom";

import { AppShell } from "@/layouts/AppShell/AppShell";
import { ChannelsPage } from "@/pages/Channels/ChannelsPage";
import { DiscoveryPage } from "@/pages/Discovery/DiscoveryPage";
import { MarketPage } from "@/pages/Market/MarketPage";
import { ChatHomePage } from "@/pages/ChatHome/ChatHomePage";
import { StudioPage } from "@/pages/Studio/StudioPage";
import { WorkspacePage } from "@/pages/Workspace/WorkspacePage";

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
        element: <WorkspacePage />,
        path: "workspace",
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
