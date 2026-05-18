import { OpsManualsPage } from "./OpsManualsPage";

type ExperiencePackFallbackEnv = {
  DEV?: boolean;
  MODE?: string;
};

export function shouldUseExperiencePackFixtureFallback(env: ExperiencePackFallbackEnv) {
  return Boolean(env.DEV || env.MODE === "test");
}

export function ExperiencePacksPage() {
  return <OpsManualsPage />;
}
