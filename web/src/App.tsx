import { Providers } from "@/app/providers";
import { AppRouter } from "@/router";

export default function App() {
  return (
    <Providers>
      <AppRouter />
    </Providers>
  );
}
