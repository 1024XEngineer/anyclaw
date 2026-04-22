import { RouterProvider } from "react-router-dom";

import { AppProviders } from "./providers";
import { router } from "./router";

export default function App() {
  return (
    <AppProviders>
      <RouterProvider router={router} />
    </AppProviders>
  );
}
