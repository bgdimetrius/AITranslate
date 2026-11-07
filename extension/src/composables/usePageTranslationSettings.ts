import { ref } from "vue";
import { storage } from "wxt/utils/storage";

const TARGET_MIRROR_KEY = "settings_target_lang";
const LEGACY_TARGET_MIRROR_KEY = "page-translation-target";
const ENABLED_MIRROR_KEY = "context_menu_enabled";

const storedTarget = storage.defineItem<string>("local:settings_target_lang");
const legacyStoredTarget = storage.defineItem<string>(
  "local:page-translation-target",
);
const storedEnabled = storage.defineItem<boolean>("local:context_menu_enabled");

function readMirroredTarget() {
  try {
    return (
      localStorage.getItem(TARGET_MIRROR_KEY) ??
      localStorage.getItem(LEGACY_TARGET_MIRROR_KEY) ??
      ""
    );
  } catch {
    return "";
  }
}

function readMirroredEnabled() {
  try {
    const value = localStorage.getItem(ENABLED_MIRROR_KEY);
    return value === null ? true : value === "true";
  } catch {
    return true;
  }
}

const target = ref(readMirroredTarget());
const enabled = ref(readMirroredEnabled());

let hydration: Promise<void> | undefined;
let targetChanged = false;
let enabledChanged = false;

function hydrate() {
  hydration ??= Promise.all([
    storedTarget.getValue(),
    legacyStoredTarget.getValue(),
    storedEnabled.getValue(),
  ])
    .then(([savedTarget, legacySavedTarget, savedEnabled]) => {
      savedTarget ??= legacySavedTarget;
      if (savedTarget && !targetChanged) target.value = savedTarget;
      if (savedEnabled !== null && !enabledChanged) {
        enabled.value = savedEnabled;
      }
    })
    .catch((error) => {
      console.error("Failed to read the page translation settings", error);
    });

  return hydration;
}

async function saveTarget(value?: string) {
  if (value !== undefined) target.value = value;
  targetChanged = true;

  try {
    localStorage.setItem(TARGET_MIRROR_KEY, target.value);
  } catch (error) {
    console.error("Failed to mirror the page translation target", error);
  }

  await storedTarget.setValue(target.value);
}

async function saveEnabled(value?: boolean) {
  if (value !== undefined) enabled.value = value;
  enabledChanged = true;

  try {
    localStorage.setItem(ENABLED_MIRROR_KEY, String(enabled.value));
  } catch (error) {
    console.error("Failed to mirror the page translation switch", error);
  }

  await storedEnabled.setValue(enabled.value);
}

async function save() {
  await Promise.all([saveTarget(), saveEnabled()]);
}

export function usePageTranslationSettings() {
  return {
    target,
    enabled,
    hydrate,
    save,
    saveTarget,
    saveEnabled,
  };
}
