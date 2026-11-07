import { computed, shallowRef, triggerRef } from "vue";
import type { TranslationAssessment } from "@/types/api";

export interface TranslationHistoryEntry {
  sourceText: string;
  translatedText: string;
  translationId: string;
  requestedSourceLanguage: string;
  detectedSourceLanguage: string;
  targetLanguage: string;
  assessment: TranslationAssessment | null;
  feedbackRevision: number;
}

interface ListNode {
  entry: TranslationHistoryEntry;
  next: ListNode | null;
  prev: ListNode | null;
}

const MAX_HISTORY_SIZE = 10;

export function useTranslationHistory() {
  const head = shallowRef<ListNode | null>(null);
  const tail = shallowRef<ListNode | null>(null);
  const currentNode = shallowRef<ListNode | null>(null);
  const size = shallowRef(0);

  const current = computed(() => currentNode.value?.entry ?? null);
  const canGoBack = computed(() => Boolean(currentNode.value?.prev));
  const canGoForward = computed(() => Boolean(currentNode.value?.next));

  function appendNode(entry: TranslationHistoryEntry) {
    const node: ListNode = {
      entry: { ...entry },
      next: null,
      prev: tail.value,
    };

    if (tail.value) tail.value.next = node;
    else head.value = node;

    tail.value = node;
    size.value += 1;
    return node;
  }

  function removeHead() {
    if (!head.value) return;

    const next = head.value.next;
    if (next) next.prev = null;
    else tail.value = null;

    head.value.next = null;
    head.value = next;
    size.value -= 1;
  }

  function removeNodesAfter(node: ListNode) {
    let removed = node.next;
    node.next = null;
    tail.value = node;

    while (removed) {
      const next = removed.next;
      removed.prev = null;
      removed.next = null;
      size.value -= 1;
      removed = next;
    }
  }

  function add(sourceText: string, translatedText?: string) {
    if (!sourceText.trim()) return current.value;

    if (currentNode.value?.entry.sourceText === sourceText) {
      if (translatedText !== undefined) {
        currentNode.value.entry.translatedText = translatedText;
        triggerRef(currentNode);
      }
      return current.value;
    }

    if (currentNode.value?.next) removeNodesAfter(currentNode.value);

    currentNode.value = appendNode({
      sourceText,
      translatedText: translatedText ?? "",
      translationId: "",
      requestedSourceLanguage: "",
      detectedSourceLanguage: "",
      targetLanguage: "",
      assessment: null,
      feedbackRevision: 0,
    });
    while (size.value > MAX_HISTORY_SIZE) removeHead();

    return current.value;
  }

  function updateTranslation(translatedText: string) {
    if (!currentNode.value) return;

    currentNode.value.entry.translatedText = translatedText;
    triggerRef(currentNode);
  }

  function updateEntry(
    entry: TranslationHistoryEntry,
    changes: Partial<Omit<TranslationHistoryEntry, "sourceText">>,
  ) {
    Object.assign(entry, changes);
    if (currentNode.value?.entry === entry) triggerRef(currentNode);
  }

  function updateCurrent(
    changes: Partial<Omit<TranslationHistoryEntry, "sourceText">>,
  ) {
    if (!currentNode.value) return;
    updateEntry(currentNode.value.entry, changes);
  }

  function goBack() {
    if (!currentNode.value?.prev) return null;

    currentNode.value = currentNode.value.prev;
    return current.value;
  }

  function goForward() {
    if (!currentNode.value?.next) return null;

    currentNode.value = currentNode.value.next;
    return current.value;
  }

  return {
    current,
    canGoBack,
    canGoForward,
    add,
    updateTranslation,
    updateEntry,
    updateCurrent,
    goBack,
    goForward,
  };
}
