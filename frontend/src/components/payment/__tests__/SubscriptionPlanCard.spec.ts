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
        weeks: "weeks",
        months: "months",
        years: "years",
        perMonth: "month",
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
        renewNow: "Renew now",
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

  // #4607：管理端保存的单位是复数（months/weeks），此前用户侧只匹配单数
  // 'month'，「1 个月」的套餐卡片被显示成「1天」。测试环境的 vue-i18n 为
  // runtime-only 构建，t() 原样返回 key，故按 key 断言单位分支。
  it("renders plural admin-form validity units instead of mislabeled days (#4607)", () => {
    expect(mountPlanCard("openai", { validity_days: 1, validity_unit: "months" }).text()).toContain("/ payment.perMonth");
    expect(mountPlanCard("openai", { validity_days: 3, validity_unit: "months" }).text()).toContain("/ 3payment.months");
    expect(mountPlanCard("openai", { validity_days: 2, validity_unit: "weeks" }).text()).toContain("/ 2payment.weeks");
    expect(mountPlanCard("openai", { validity_days: 1, validity_unit: "years" }).text()).toContain("/ 1payment.years");
    expect(mountPlanCard("openai", { validity_days: 30, validity_unit: "day" }).text()).toContain("/ 30payment.days");
  });

  it("uses the configured currency symbol while preserving USD for legacy plans", () => {
    const cnyPlan = mountPlanCard("openai", { currency: "CNY", original_price: 20 }).text();

    expect(cnyPlan).toContain("¥10CNY");
    expect(cnyPlan).toContain("¥20CNY");
    expect(mountPlanCard("openai", { currency: "USD" }).text()).toContain("$10USD");
    expect(mountPlanCard("openai", { currency: "HKD" }).text()).toContain("HK$10HKD");
    expect(mountPlanCard("openai", { currency: "" }).text()).toContain("¥10");
  });

  it("marks a plan as renewal when its non-primary group is covered by an entitlement", () => {
    const wrapper = mount(SubscriptionPlanCard, {
      props: {
        plan: {
          id: 3,
          group_id: 10,
          groups: [
            { id: 10, name: "OpenAI", platform: "openai", rate_multiplier: 1 },
            { id: 11, name: "Claude", platform: "anthropic", rate_multiplier: 1 },
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
        activeSubscriptions: [{
          id: -99,
          user_id: 0,
          group_id: 10,
          status: "active",
          starts_at: "2026-06-01T00:00:00Z",
          expires_at: "2099-07-01T00:00:00Z",
          daily_usage_usd: 0,
          weekly_usage_usd: 0,
          monthly_usage_usd: 0,
          daily_window_start: null,
          weekly_window_start: null,
          monthly_window_start: null,
          created_at: "2026-06-01T00:00:00Z",
          updated_at: "2026-06-01T00:00:00Z",
          entitlement_only: true,
          entitlement_id: 99,
          plan_id: 3,
          plan_name: "Multi",
          groups: [
            { id: 10, name: "OpenAI", platform: "openai", rate_multiplier: 1 },
            { id: 11, name: "Claude", platform: "anthropic", rate_multiplier: 1 },
          ],
        }],
      },
      global: { plugins: [i18n, createPinia()] },
    });

    expect(wrapper.text()).toContain("payment.renewNow");
  });

  it("does not mark a different plan as renewal just because a group overlaps", () => {
    const wrapper = mount(SubscriptionPlanCard, {
      props: {
        plan: {
          id: 3,
          group_id: 10,
          groups: [{ id: 10, name: "OpenAI", platform: "openai", rate_multiplier: 1 }],
          name: "Target",
          price: 10,
          features: [],
          rate_multiplier: 1,
          validity_days: 30,
          validity_unit: "day",
          access_scope: "explicit",
          is_active: true,
        },
        activeSubscriptions: [{
          id: -99,
          user_id: 0,
          group_id: 10,
          status: "active",
          starts_at: "2026-06-01T00:00:00Z",
          expires_at: "2099-07-01T00:00:00Z",
          daily_usage_usd: 0,
          weekly_usage_usd: 0,
          monthly_usage_usd: 0,
          daily_window_start: null,
          weekly_window_start: null,
          monthly_window_start: null,
          created_at: "2026-06-01T00:00:00Z",
          updated_at: "2026-06-01T00:00:00Z",
          entitlement_only: true,
          entitlement_id: 99,
          plan_id: 4,
          groups: [{ id: 10, name: "OpenAI", platform: "openai", rate_multiplier: 1 }],
        }],
      },
      global: { plugins: [i18n, createPinia()] },
    });

    expect(wrapper.text()).toContain("payment.subscribeNow");
    expect(wrapper.text()).not.toContain("payment.renewNow");
  });
});
