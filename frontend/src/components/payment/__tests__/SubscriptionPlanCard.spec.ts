import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { createPinia } from "pinia";
import { createI18n } from "vue-i18n";
import type { SubscriptionPlan } from "@/types/payment";
import SubscriptionPlanCard from "../SubscriptionPlanCard.vue";

const i18n = createI18n({
  legacy: false,
  locale: "en",
  fallbackWarn: false,
  missingWarn: false,
  messages: {
    en: {
      payment: {
        days: "days",
        models: "Models",
        planCard: {
          allIncluded: "All included",
          authorizedGroups: "Authorized groups",
          dailyLimit: "Daily",
          groupsCount: "{count} groups",
          noGroups: "No authorized groups",
          overageBalanceFallback: "Fallback to balance when exhausted",
          overageBlock: "Block when quota is exhausted",
          overagePolicy: "Overage",
          priceUnavailable: "Price pending sync",
          quota: "Quota",
          rate: "Rate",
          unitCost: "Effective cost",
          unitCostValue: "{amount}/$1",
          unlimited: "Unlimited",
          validity: "Validity",
          weeklyLimit: "Weekly",
          monthlyLimit: "Monthly",
        },
        subscribeNow: "Subscribe now",
      },
    },
  },
});

const mountPlanCard = (groupPlatform: string, overrides: Partial<SubscriptionPlan> = {}) =>
  mount(SubscriptionPlanCard, {
    props: {
      plan: {
        id: 1,
        group_id: 10,
        group_platform: groupPlatform,
        name: "Pro",
        price: 10,
        amount: 1000,
        features: [],
        rate_multiplier: 1,
        validity_days: 30,
        validity_unit: "day",
        supported_model_scopes: ["claude", "gemini_text", "gemini_image"],
        is_active: true,
        ...overrides,
      },
    },
    global: { plugins: [i18n, createPinia()] },
  });

describe("SubscriptionPlanCard", () => {
  it("does not show Antigravity model scopes for OpenAI plans", () => {
    const text = mountPlanCard("openai").text();

    expect(text).not.toContain("Claude");
    expect(text).not.toContain("Gemini");
    expect(text).not.toContain("Imagen");
  });

  it("shows model scopes for Antigravity plans", () => {
    const text = mountPlanCard("antigravity").text();

    expect(text).toContain("Claude");
    expect(text).toContain("Gemini");
    expect(text).toContain("Imagen");
  });

  it("labels multi-group plans as all included instead of a single platform", () => {
    const wrapper = mount(SubscriptionPlanCard, {
      props: {
        plan: {
          id: 2,
          group_id: 0,
          groups: [
            {
              id: 20,
              name: "Claude group",
              platform: "anthropic",
              rate_multiplier: 1,
              sort_order: 0,
            },
            {
              id: 21,
              name: "OpenAI group",
              platform: "openai",
              rate_multiplier: 1,
              sort_order: 1,
            },
          ],
          name: "Multi",
          description: "",
          price: 10,
          features: [],
          rate_multiplier: 1,
          validity_days: 30,
          validity_unit: "day",
          access_scope: "explicit",
          is_active: true,
        },
      },
      global: { plugins: [i18n, createPinia()] },
    });

    expect(wrapper.find(".rounded-full").text()).toBe("payment.planCard.allIncluded");
  });

  it("uses the configured currency symbol while preserving USD for legacy plans", () => {
    const cnyPlan = mountPlanCard("openai", { currency: "CNY", original_price: 20 }).text();

    expect(cnyPlan).toContain("¥10CNY");
    expect(cnyPlan).toContain("¥20CNY");
    expect(mountPlanCard("openai", { currency: "USD" }).text()).toContain("$10USD");
    expect(mountPlanCard("openai", { currency: "HKD" }).text()).toContain("HK$10HKD");
    expect(mountPlanCard("openai", { currency: "" }).text()).toContain("¥10");
  });
});
