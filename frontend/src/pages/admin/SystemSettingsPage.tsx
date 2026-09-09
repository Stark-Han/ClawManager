import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { AlertTriangle, KeyRound, Pencil, Plus, Rocket, Save, ShieldCheck, Trash2 } from 'lucide-react';
import AdminLayout from '../../components/AdminLayout';
import { useI18n } from '../../contexts/I18nContext';
import PasswordSettingsSection from '../../components/PasswordSettingsSection';
import {
  systemSettingsService,
  type SystemImageSetting,
} from '../../services/systemSettingsService';
import {
  enterpriseAuthService,
  type EnterpriseAuthConfig,
  type EnterpriseAuthConfigUpdate,
  type EnterpriseAuthStatus,
  type LDAPConfigPublic,
} from '../../services/enterpriseAuthService';
import { runtimePoolService } from '../../services/runtimePoolService';
import type { RuntimeUpgradeDetails, RuntimeUpgradePreflightResult } from '../../types/runtimePool';
import type { RuntimePod, RuntimeType } from '../../types/runtimePool';
import { localizeEnterpriseAuthIssue, localizeEnterpriseAuthIssues } from '../../utils/enterpriseAuthErrors';

type ImageRuntimeType = 'desktop' | 'gateway';
type RuntimeGroup = 'lite' | 'pro';
type RuntimeVariant = 'linux' | 'windows';

interface RuntimeCardDefinition {
  instance_type: string;
  runtime_type: ImageRuntimeType;
  display_name: string;
  image: string;
  runtime_variant?: RuntimeVariant;
}

const LITE_RUNTIME_CARDS: RuntimeCardDefinition[] = [
  {
    instance_type: 'openclaw',
    runtime_type: 'gateway',
    display_name: 'OpenClaw Lite',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/openclaw-lite:latest',
  },
  {
    instance_type: 'hermes',
    runtime_type: 'gateway',
    display_name: 'Hermes Lite',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/hermes-lite:latest',
  },
  {
    instance_type: 'opencode',
    runtime_type: 'gateway',
    display_name: 'OpenCode Lite',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/opencode-lite:latest',
  },
  {
    instance_type: 'deepseek-harness',
    runtime_type: 'gateway',
    display_name: 'DeepSeek Harness Lite',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/deepseek-harness-lite:latest',
  },
];

const PRO_BASE_RUNTIME_CARDS: RuntimeCardDefinition[] = [
  {
    instance_type: 'openclaw',
    runtime_type: 'desktop',
    display_name: 'OpenClaw Pro',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/openclaw:latest',
  },
  {
    instance_type: 'hermes',
    runtime_type: 'desktop',
    display_name: 'Hermes Pro',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/hermes:latest',
  },
  {
    instance_type: 'deepseek-harness',
    runtime_type: 'desktop',
    display_name: 'DeepSeek Harness Pro',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/deepseek-harness:latest',
  },
  {
    instance_type: 'codex',
    runtime_type: 'desktop',
    runtime_variant: 'windows',
    display_name: 'Codex Pro',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/windows-vm-codex:latest',
  },
  {
    instance_type: 'claude-code',
    runtime_type: 'desktop',
    display_name: 'Claude Code Pro',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/claude-code:latest',
  },
  {
    instance_type: 'workbuddy',
    runtime_type: 'desktop',
    runtime_variant: 'linux',
    display_name: 'Workbuddy Pro',
    image: 'ghcr.io/yuan-lab-llm/agentsruntime/workbuddy-linux:latest',
  },
];

const RUNTIME_VARIANT_IMAGES: Record<'workbuddy' | 'codex', Record<RuntimeVariant, string>> = {
  workbuddy: {
    linux: 'ghcr.io/yuan-lab-llm/agentsruntime/workbuddy-linux:latest',
    windows: 'ghcr.io/yuan-lab-llm/agentsruntime/windows-vm-workbuddy:latest',
  },
  codex: {
    linux: 'ghcr.io/yuan-lab-llm/agentsruntime/codex:latest',
    windows: 'ghcr.io/yuan-lab-llm/agentsruntime/windows-vm-codex:latest',
  },
};
const PRO_CUSTOM_DEFAULT_IMAGE = 'registry.example.com/your-custom-image:latest';
// The team distribution does not ship these products. Their saved settings
// remain intact for existing-instance compatibility, but are not exposed as
// configurable or creatable runtime cards.
const HIDDEN_TEAM_RUNTIME_CARD_TYPES = new Set(['workbuddy', 'codex', 'claude-code']);
const TEMPORARILY_HIDDEN_RUNTIME_CARD_VARIANTS = new Set(['workbuddy:windows']);
const VISIBLE_PRO_BASE_RUNTIME_CARDS = PRO_BASE_RUNTIME_CARDS.filter(
  (card) => !HIDDEN_TEAM_RUNTIME_CARD_TYPES.has(card.instance_type),
);
const FIXED_RUNTIME_CARDS = [...LITE_RUNTIME_CARDS, ...VISIBLE_PRO_BASE_RUNTIME_CARDS];
const DEFAULT_LDAP_FORM: LDAPConfigPublic = {
  host: '',
  port: 389,
  use_tls: false,
  start_tls: false,
  skip_tls_verify: false,
  tls_ca_file: '',
  tls_server_name: '',
  bind_dn: '',
  base_dn: '',
  user_filter: '(&(objectClass=inetOrgPerson)(uid=%s))',
  username_attribute: 'uid',
  email_attribute: 'mail',
  group_base_dn: '',
  group_filter: '(member=%s)',
  admin_group_dns: [],
  default_role: 'user',
};

const LDAP_PLACEHOLDERS = {
  host: 'ldap.example.com',
  tlsCAFile: '/etc/ssl/certs/company-ldap.pem',
  tlsServerName: 'ldap.internal.example.com',
  bindDN: 'cn=readonly,dc=example,dc=com',
  baseDN: 'ou=People,dc=example,dc=com',
  groupBaseDN: 'ou=Groups,dc=example,dc=com',
  userFilter: '(&(objectClass=inetOrgPerson)(uid=%s))',
  groupFilter: '(member=%s)',
  adminGroups: 'cn=clawmanager-admins,ou=Groups,dc=example,dc=com',
};

interface EditableImageCard extends SystemImageSetting {
  local_id: string;
  runtime_type: ImageRuntimeType;
  default_image?: string;
  group: RuntimeGroup;
  isBase?: boolean;
  isNew?: boolean;
  saving?: boolean;
  error?: string | null;
}

function getErrorMessage(error: unknown, fallback: string, translateEnterpriseIssue?: (message: string) => string) {
  const responseError = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
  if (responseError) {
    return translateEnterpriseIssue ? translateEnterpriseIssue(responseError) : responseError;
  }
  if (error instanceof Error) {
    return translateEnterpriseIssue ? translateEnterpriseIssue(error.message) : error.message;
  }
  return fallback;
}

function normalizeImageRuntimeType(runtimeType?: string): ImageRuntimeType {
  return runtimeType === 'gateway' || runtimeType === 'shell' ? 'gateway' : 'desktop';
}

function fixedCardKey(card: Pick<SystemImageSetting, 'instance_type' | 'runtime_type'>) {
  return `${card.instance_type}:${normalizeImageRuntimeType(card.runtime_type)}`;
}

function defaultForCard(card: Pick<SystemImageSetting, 'instance_type' | 'runtime_type'>) {
  return FIXED_RUNTIME_CARDS.find((item) => fixedCardKey(item) === fixedCardKey(card));
}

function groupForRuntimeType(runtimeType: ImageRuntimeType): RuntimeGroup {
  return runtimeType === 'gateway' ? 'lite' : 'pro';
}

function supportsRuntimeVariant(instanceType: string): instanceType is 'workbuddy' | 'codex' {
  return instanceType === 'workbuddy' || instanceType === 'codex';
}

function canSelectRuntimeVariant(instanceType: string): instanceType is 'codex' {
  return instanceType === 'codex';
}

function inferRuntimeVariant(instanceType: string, image?: string): RuntimeVariant {
  const normalizedImage = image?.trim().toLowerCase() ?? '';
  if (instanceType === 'workbuddy' && normalizedImage.includes('workbuddy-linux')) {
    return 'linux';
  }
  if (instanceType === 'codex' && normalizedImage.includes('agentsruntime/codex')) {
    return 'linux';
  }
  return 'windows';
}

function runtimeVariantForCard(item: SystemImageSetting, definition?: RuntimeCardDefinition): RuntimeVariant | undefined {
  if (!supportsRuntimeVariant(item.instance_type)) return undefined;
  return item.runtime_variant ?? definition?.runtime_variant ?? inferRuntimeVariant(item.instance_type, item.image);
}

function isRuntimeCardVisible(item: SystemImageSetting) {
  if (HIDDEN_TEAM_RUNTIME_CARD_TYPES.has(item.instance_type)) return false;
  const runtimeVariant = runtimeVariantForCard(item);
  return !runtimeVariant || !TEMPORARILY_HIDDEN_RUNTIME_CARD_VARIANTS.has(`${item.instance_type}:${runtimeVariant}`);
}

function defaultImageForVariant(instanceType: string, variant?: RuntimeVariant) {
  return supportsRuntimeVariant(instanceType) && variant
    ? RUNTIME_VARIANT_IMAGES[instanceType][variant]
    : undefined;
}

function runtimePodSeenAt(pod: RuntimePod) {
  const parsed = Date.parse(pod.last_seen_at || pod.updated_at || '');
  return Number.isNaN(parsed) ? 0 : parsed;
}

function resolveCurrentRuntimeImage(pods: RuntimePod[]) {
  const candidates = pods
    .filter((pod) => pod.image_ref?.trim())
    .filter((pod) => !pod.draining && pod.state !== 'deleted')
	.filter((pod) => pod.pool_purpose !== 'openclaw-upgrade-lab' && pod.pool_role !== 'upgrade-lab')
	.filter((pod) => pod.pool_role !== 'upgrade-target' || pod.scheduling_enabled === true || pod.used_slots > 0)
	.filter((pod) => pod.scheduling_enabled !== false || pod.used_slots > 0)
    .sort((a, b) => runtimePodSeenAt(b) - runtimePodSeenAt(a) || b.id - a.id);
  if (candidates.length === 0) {
    return '';
  }

	const imagesByDigest = new Map<string, string>();
	for (const pod of candidates) {
		const key = pod.image_digest?.trim() || pod.image_ref.trim();
		if (!imagesByDigest.has(key)) {
			imagesByDigest.set(key, pod.image_ref.trim());
		}
	}
	const images = Array.from(imagesByDigest.values());
  return images.join(', ');
}

function ldapConfigWithDefaults(ldap?: Partial<LDAPConfigPublic>): LDAPConfigPublic {
  const nonEmpty = (value: string | undefined, fallback: string) =>
    value?.trim() ? value : fallback;

  return {
    ...DEFAULT_LDAP_FORM,
    ...ldap,
    port: ldap?.port || (ldap?.use_tls ? 636 : 389),
    bind_dn: nonEmpty(ldap?.bind_dn, DEFAULT_LDAP_FORM.bind_dn),
    base_dn: nonEmpty(ldap?.base_dn, DEFAULT_LDAP_FORM.base_dn),
    user_filter: nonEmpty(ldap?.user_filter, DEFAULT_LDAP_FORM.user_filter),
    username_attribute: nonEmpty(ldap?.username_attribute, DEFAULT_LDAP_FORM.username_attribute),
    email_attribute: nonEmpty(ldap?.email_attribute, DEFAULT_LDAP_FORM.email_attribute),
    group_base_dn: nonEmpty(ldap?.group_base_dn, DEFAULT_LDAP_FORM.group_base_dn),
    group_filter: nonEmpty(ldap?.group_filter, DEFAULT_LDAP_FORM.group_filter),
    admin_group_dns: ldap?.admin_group_dns?.length
      ? ldap.admin_group_dns
      : [...DEFAULT_LDAP_FORM.admin_group_dns],
    default_role: nonEmpty(ldap?.default_role, DEFAULT_LDAP_FORM.default_role),
  };
}

function parseAdminGroupDNs(value: string) {
  return value.split(/\r?\n|;/).map((item) => item.trim()).filter(Boolean);
}

function enterpriseStatusFailed(status?: EnterpriseAuthStatus | null) {
  if (!status) {
    return false;
  }
  return Boolean(status.error) || Object.values(status.checks || {}).includes('failed');
}

function toEditableCard(
  item: SystemImageSetting,
  index: number,
  fallback?: RuntimeCardDefinition,
): EditableImageCard {
  const runtimeType = normalizeImageRuntimeType(item.runtime_type);
  const definition = fallback ?? defaultForCard({ ...item, runtime_type: runtimeType });
  const runtimeVariant = runtimeVariantForCard(item, definition);
  const variantDefaultImage = defaultImageForVariant(item.instance_type, runtimeVariant);
  return {
    ...item,
    runtime_type: runtimeType,
    runtime_variant: runtimeVariant,
    display_name: item.display_name || definition?.display_name || 'Custom Pro',
    image: item.image || definition?.image || PRO_CUSTOM_DEFAULT_IMAGE,
    default_image: variantDefaultImage ?? definition?.image,
    group: groupForRuntimeType(runtimeType),
    isBase: Boolean(definition),
    isNew: !item.id,
    local_id: item.id ? `image-${item.id}` : `image-${item.instance_type}-${runtimeType}-${index}`,
    error: null,
  };
}

function buildRuntimeCards(items: SystemImageSetting[]): EditableImageCard[] {
  const enabledCards = items
    .filter((item) => item.is_enabled !== false && isRuntimeCardVisible(item))
    .map((item, index) => toEditableCard(item, index));
  const byFixedKey = new Map(enabledCards.map((card) => [fixedCardKey(card), card]));

  const fixedCards = FIXED_RUNTIME_CARDS.map((definition, index) => {
    const existing = byFixedKey.get(fixedCardKey(definition));
    if (existing) {
      return {
        ...existing,
        display_name: definition.display_name,
        default_image: defaultImageForVariant(existing.instance_type, existing.runtime_variant) ?? definition.image,
        group: groupForRuntimeType(definition.runtime_type),
        isBase: true,
      };
    }

    return toEditableCard(
      {
        instance_type: definition.instance_type,
        runtime_type: definition.runtime_type,
        runtime_variant: definition.runtime_variant,
        display_name: definition.display_name,
        image: definition.image,
        is_enabled: true,
      },
      index,
      definition,
    );
  });

  const customProCards = enabledCards.filter((card) =>
    card.runtime_type === 'desktop' && !defaultForCard(card),
  ).map((card) => ({
    ...card,
    group: 'pro' as const,
    isBase: false,
    default_image: card.default_image ?? PRO_CUSTOM_DEFAULT_IMAGE,
  }));

  return [...fixedCards, ...customProCards];
}

const SystemSettingsPage: React.FC = () => {
  const { t } = useI18n();
  const [cards, setCards] = useState<EditableImageCard[]>([]);
  const [loading, setLoading] = useState(true);
  const [pageError, setPageError] = useState<string | null>(null);
  const [rolloutRuntimeType, setRolloutRuntimeType] = useState<RuntimeType>('openclaw');
  const [rolloutImage, setRolloutImage] = useState('');
  const [rolloutCurrentImage, setRolloutCurrentImage] = useState('');
  const [rolloutCurrentLoading, setRolloutCurrentLoading] = useState(false);
	const [rolloutBatchSize, setRolloutBatchSize] = useState(2);
  const [rolloutMaxUnavailable, setRolloutMaxUnavailable] = useState(1);
  const [rolloutSaving, setRolloutSaving] = useState(false);
  const [rolloutError, setRolloutError] = useState<string | null>(null);
  const [rolloutPreflight, setRolloutPreflight] = useState<RuntimeUpgradePreflightResult | null>(null);
	const [rolloutDetails, setRolloutDetails] = useState<RuntimeUpgradeDetails | null>(null);
	const [activeRolloutId, setActiveRolloutId] = useState<number | null>(null);
	const [enterpriseConfig, setEnterpriseConfig] = useState<EnterpriseAuthConfig | null>(null);
	const [enterpriseLoading, setEnterpriseLoading] = useState(true);
	const [enterpriseSaving, setEnterpriseSaving] = useState(false);
	const [enterpriseTesting, setEnterpriseTesting] = useState(false);
	const [enterpriseError, setEnterpriseError] = useState<string | null>(null);
	const [enterpriseTestStatus, setEnterpriseTestStatus] = useState<EnterpriseAuthStatus | null>(null);
	const [ldapEnabled, setLdapEnabled] = useState(false);
	const [allowLocalFallback, setAllowLocalFallback] = useState(true);
	const [syncRole, setSyncRole] = useState(false);
	const [ldapForm, setLdapForm] = useState<LDAPConfigPublic>(DEFAULT_LDAP_FORM);
	const [bindPassword, setBindPassword] = useState('');
	const [bindPasswordEditing, setBindPasswordEditing] = useState(false);
	const [clearBindPassword, setClearBindPassword] = useState(false);
	const [adminGroupDNsText, setAdminGroupDNsText] = useState('');

  const liteCards = useMemo(
    () => LITE_RUNTIME_CARDS.map((definition) =>
      cards.find((card) => fixedCardKey(card) === fixedCardKey(definition)),
    ).filter((card): card is EditableImageCard => Boolean(card)),
    [cards],
  );

  const proBaseCards = useMemo(
    () => PRO_BASE_RUNTIME_CARDS.map((definition) =>
      cards.find((card) => fixedCardKey(card) === fixedCardKey(definition)),
    ).filter((card): card is EditableImageCard => Boolean(card)),
    [cards],
  );

  const proCustomCards = useMemo(
    () => cards.filter((card) => card.group === 'pro' && !card.isBase),
    [cards],
  );

  const rolloutCard = useMemo(
    () => liteCards.find((card) => card.instance_type === rolloutRuntimeType),
    [liteCards, rolloutRuntimeType],
  );

  useEffect(() => {
    const loadSettings = async () => {
      try {
        setLoading(true);
        setPageError(null);
        const items = await systemSettingsService.getImageSettings();
        const nextCards = buildRuntimeCards(items);
        setCards(nextCards);
        const nextRolloutCard = nextCards.find(
          (card) => card.instance_type === rolloutRuntimeType && card.runtime_type === 'gateway',
        );
        setRolloutImage(
          rolloutRuntimeType === 'openclaw'
            ? ''
            : nextRolloutCard?.image.trim() || '',
        );
      } catch (error: unknown) {
        setPageError(getErrorMessage(error, t('systemSettingsPage.loadFailed')));
      } finally {
        setLoading(false);
      }
    };

    loadSettings();
  }, [t, rolloutRuntimeType]);

  useEffect(() => {
    const loadEnterpriseConfig = async () => {
      try {
        setEnterpriseLoading(true);
        setEnterpriseError(null);
        const config = await enterpriseAuthService.getConfig();
        const ldap = ldapConfigWithDefaults(config.ldap);
        setEnterpriseConfig(config);
        setLdapEnabled(config.enabled);
        setAllowLocalFallback(config.allow_local_fallback);
        setSyncRole(config.sync_role);
        setLdapForm(ldap);
        setAdminGroupDNsText(ldap.admin_group_dns.join('\n'));
        setBindPassword('');
        setBindPasswordEditing(false);
        setClearBindPassword(false);
        setEnterpriseTestStatus(null);
      } catch (error: unknown) {
        setEnterpriseError(getErrorMessage(
          error,
          t('systemSettingsPage.enterpriseLoadFailed'),
          (message) => localizeEnterpriseAuthIssue(message, t),
        ));
      } finally {
        setEnterpriseLoading(false);
      }
    };

    void loadEnterpriseConfig();
  }, [t]);

  useEffect(() => {
    let cancelled = false;
    const loadCurrentImage = async () => {
      try {
        setRolloutCurrentLoading(true);
        const pods = await runtimePoolService.listPods(rolloutRuntimeType);
        if (!cancelled) {
          setRolloutCurrentImage(resolveCurrentRuntimeImage(pods));
        }
      } catch {
        if (!cancelled) {
          setRolloutCurrentImage('');
        }
      } finally {
        if (!cancelled) {
          setRolloutCurrentLoading(false);
        }
      }
    };

    void loadCurrentImage();
    return () => {
      cancelled = true;
    };
  }, [rolloutRuntimeType]);

  const refreshRolloutCurrentImage = useCallback(async (runtimeType: RuntimeType) => {
    try {
      setRolloutCurrentLoading(true);
      const pods = await runtimePoolService.listPods(runtimeType);
      setRolloutCurrentImage(resolveCurrentRuntimeImage(pods));
    } catch {
      setRolloutCurrentImage('');
    } finally {
      setRolloutCurrentLoading(false);
    }
  }, []);

	useEffect(() => {
		if (!activeRolloutId) {
			return undefined;
		}
		let cancelled = false;
		let timer: number | undefined;
		const poll = async () => {
			try {
				const details = await runtimePoolService.getRollout(activeRolloutId);
				if (cancelled) return;
				setRolloutDetails(details);
				const terminal = ['finished', 'error', 'cancelled'].includes(details.rollout.status);
				if (terminal) {
					setActiveRolloutId(null);
					setRolloutSaving(false);
					void refreshRolloutCurrentImage(details.rollout.runtime_type);
					if (details.rollout.status === 'error') {
						setRolloutError(details.rollout.rollback_status === 'restored'
							? `升级失败，已恢复旧版本：${details.rollout.error_message || details.rollout.rollback_error || '请查看阶段详情'}`
							: details.rollout.error_message || details.rollout.rollback_error || '升级失败');
					}
					return;
				}
				timer = window.setTimeout(() => void poll(), 2000);
			} catch (error: unknown) {
				if (!cancelled) {
					setRolloutError(getErrorMessage(error, t('systemSettingsPage.rolloutFailed')));
					timer = window.setTimeout(() => void poll(), 5000);
				}
			}
		};
		void poll();
		return () => {
			cancelled = true;
			if (timer !== undefined) window.clearTimeout(timer);
		};
	}, [activeRolloutId, refreshRolloutCurrentImage, t]);

  const updateCard = (localId: string, patch: Partial<EditableImageCard>) => {
    setCards((current) => current.map((card) =>
      card.local_id === localId ? { ...card, ...patch, error: null } : card,
    ));
  };

  const updateLDAPForm = (patch: Partial<LDAPConfigPublic>) => {
    setLdapForm((current) => ({ ...current, ...patch }));
    setEnterpriseError(null);
    setEnterpriseTestStatus(null);
  };

  const handleClearBindPasswordChange = (checked: boolean) => {
    setClearBindPassword(checked);
    if (checked) {
      setBindPassword('');
      setBindPasswordEditing(false);
    }
  };

  const buildEnterprisePayload = (): EnterpriseAuthConfigUpdate => ({
    expected_version: enterpriseConfig?.version ?? 0,
    enabled: ldapEnabled,
    allow_local_fallback: allowLocalFallback,
    sync_role: syncRole,
    ldap: {
      ...ldapConfigWithDefaults(ldapForm),
      host: ldapForm.host.trim(),
      tls_ca_file: ldapForm.tls_ca_file.trim(),
      tls_server_name: ldapForm.tls_server_name.trim(),
      bind_dn: ldapForm.bind_dn.trim(),
      base_dn: ldapForm.base_dn.trim(),
      user_filter: ldapForm.user_filter.trim(),
      username_attribute: ldapForm.username_attribute.trim(),
      email_attribute: ldapForm.email_attribute.trim(),
      group_base_dn: ldapForm.group_base_dn.trim(),
      group_filter: ldapForm.group_filter.trim(),
      admin_group_dns: parseAdminGroupDNs(adminGroupDNsText),
      default_role: ldapForm.default_role === 'admin' ? 'admin' : 'user',
    },
    bind_password: clearBindPassword ? '' : bindPassword,
    clear_bind_password: clearBindPassword,
  });

  const handleTestEnterpriseConfig = async () => {
    try {
      setEnterpriseTesting(true);
      setEnterpriseError(null);
      const status = await enterpriseAuthService.testConfig(buildEnterprisePayload());
      setEnterpriseTestStatus(status);
    } catch (error: unknown) {
      setEnterpriseError(getErrorMessage(
        error,
        t('systemSettingsPage.enterpriseTestFailed'),
        (message) => localizeEnterpriseAuthIssue(message, t),
      ));
    } finally {
      setEnterpriseTesting(false);
    }
  };

  const handleSaveEnterpriseConfig = async () => {
    try {
      setEnterpriseSaving(true);
      setEnterpriseError(null);
      const saved = await enterpriseAuthService.updateConfig(buildEnterprisePayload());
      const ldap = ldapConfigWithDefaults(saved.ldap);
      setEnterpriseConfig(saved);
      setLdapEnabled(saved.enabled);
      setAllowLocalFallback(saved.allow_local_fallback);
      setSyncRole(saved.sync_role);
      setLdapForm(ldap);
      setAdminGroupDNsText(ldap.admin_group_dns.join('\n'));
      setBindPassword('');
      setBindPasswordEditing(false);
      setClearBindPassword(false);
      setEnterpriseTestStatus(saved.status);
    } catch (error: unknown) {
      setEnterpriseError(getErrorMessage(
        error,
        t('systemSettingsPage.enterpriseSaveFailed'),
        (message) => localizeEnterpriseAuthIssue(message, t),
      ));
    } finally {
      setEnterpriseSaving(false);
    }
  };

  const addProCustomCard = () => {
    setCards((current) => [
      ...current,
      {
        local_id: `new-pro-custom-${Date.now()}`,
        instance_type: 'custom',
        runtime_type: 'desktop',
        display_name: 'Custom Pro',
        image: PRO_CUSTOM_DEFAULT_IMAGE,
        default_image: PRO_CUSTOM_DEFAULT_IMAGE,
        group: 'pro',
        isBase: false,
        isNew: true,
        is_enabled: true,
        error: null,
      },
    ]);
  };

  const saveCard = async (card: EditableImageCard) => {
    if (!card.instance_type || !card.image.trim()) {
      updateCard(card.local_id, { error: t('systemSettingsPage.requiredFields') });
      return;
    }

    const normalizedImage = card.image.trim().toLowerCase();
    const duplicate = cards.some((item) =>
      item.local_id !== card.local_id &&
      item.instance_type === card.instance_type &&
      item.runtime_type === card.runtime_type &&
      item.image.trim().toLowerCase() === normalizedImage,
    );
    if (duplicate) {
      updateCard(card.local_id, { error: t('systemSettingsPage.duplicateImage') });
      return;
    }

    updateCard(card.local_id, { saving: true, error: null });

    try {
      const saved = await systemSettingsService.saveImageSetting({
        id: card.id,
        instance_type: card.instance_type,
        runtime_type: card.runtime_type,
        runtime_variant: card.runtime_variant,
        display_name: card.display_name.trim() || (card.isBase ? card.display_name : 'Custom Pro'),
        image: card.image.trim(),
      });
      const nextCard = toEditableCard(
        {
          ...saved,
          runtime_type: normalizeImageRuntimeType(saved.runtime_type),
        },
        0,
        defaultForCard(saved),
      );

      setCards((current) => current.map((item) => item.local_id === card.local_id ? {
        ...item,
        ...nextCard,
        local_id: item.local_id,
        isNew: false,
        saving: false,
        error: null,
      } : item));

      if (nextCard.runtime_type === 'gateway' && nextCard.instance_type === rolloutRuntimeType) {
        setRolloutImage(nextCard.image.trim());
      }
    } catch (error: unknown) {
      updateCard(card.local_id, {
        saving: false,
        error: getErrorMessage(error, t('systemSettingsPage.saveFailed')),
      });
    }
  };

  const updateRuntimeVariant = (card: EditableImageCard, runtimeVariant: RuntimeVariant) => {
    const previousDefault = card.default_image;
    const nextDefault = defaultImageForVariant(card.instance_type, runtimeVariant);
    updateCard(card.local_id, {
      runtime_variant: runtimeVariant,
      default_image: nextDefault,
      image: !card.image.trim() || card.image.trim() === previousDefault ? nextDefault ?? card.image : card.image,
    });
  };

  const deleteCard = async (card: EditableImageCard) => {
    if (card.isBase) {
      return;
    }
    if (card.isNew) {
      setCards((current) => current.filter((item) => item.local_id !== card.local_id));
      return;
    }

    updateCard(card.local_id, { saving: true, error: null });
    try {
      await systemSettingsService.deleteImageSetting(card.id ?? card.instance_type);
      setCards((current) => current.filter((item) => item.local_id !== card.local_id));
    } catch (error: unknown) {
      updateCard(card.local_id, {
        saving: false,
        error: getErrorMessage(error, t('systemSettingsPage.deleteFailed')),
      });
    }
  };

  const handleRolloutRuntimeTypeChange = (runtimeType: RuntimeType) => {
    const nextCard = liteCards.find((card) => card.instance_type === runtimeType);
    setRolloutRuntimeType(runtimeType);
    setRolloutImage(runtimeType === 'openclaw'
      ? ''
      : nextCard?.image.trim() || LITE_RUNTIME_CARDS.find((item) => item.instance_type === runtimeType)?.image || '');
    setRolloutError(null);
    setRolloutPreflight(null);
		setRolloutDetails(null);
    setRolloutMaxUnavailable(1);
  };

  const startRollout = async () => {
    if (!rolloutImage.trim()) {
      setRolloutError(t('systemSettingsPage.rolloutTargetRequired'));
      return;
    }
    try {
      setRolloutSaving(true);
      setRolloutError(null);
		let targetImage = rolloutImage.trim();
		let preflight = rolloutPreflight;
		if (rolloutRuntimeType === 'openclaw' && preflight && !preflight.passed) {
			setRolloutError(preflight.blockers.join('；'));
			return;
		}
      if (rolloutRuntimeType === 'openclaw' && !preflight) {
			preflight = await runtimePoolService.preflightOpenClawRollout({
			  target_image_ref: targetImage,
			  batch_size: Math.max(1, rolloutBatchSize),
			  max_unavailable: 0,
			  auto_rollback: true,
			});
			targetImage = preflight.target_image_ref || targetImage;
			setRolloutImage(targetImage);
			if (!preflight.passed) {
			  setRolloutPreflight(preflight);
			  setRolloutError(preflight.blockers.join('；'));
			  return;
			}
			if (preflight.strategy === 'openclaw_8plus_data_safe') {
			  setRolloutPreflight(preflight);
			  setRolloutMaxUnavailable(0);
			  return;
			}
		}
		const rollout = await runtimePoolService.startRollout({
        runtime_type: rolloutRuntimeType,
		target_image_ref: targetImage,
        batch_size: Math.max(1, rolloutBatchSize),
		max_unavailable: preflight?.strategy === 'openclaw_8plus_data_safe' ? 0 : Math.max(1, rolloutMaxUnavailable),
		preflight_id: preflight?.strategy === 'openclaw_8plus_data_safe' ? preflight.rollout?.preflight_id : undefined,
        auto_rollback: true,
      });
		setRolloutDetails({ rollout, items: [], audits: [] });
		setActiveRolloutId(rollout.id);
      setRolloutPreflight(null);
      setRolloutImage(rolloutCard?.image.trim() || rolloutImage.trim());
      void refreshRolloutCurrentImage(rolloutRuntimeType);
    } catch (error: unknown) {
      setRolloutError(getErrorMessage(error, t('systemSettingsPage.rolloutFailed')));
    } finally {
		if (!activeRolloutId) setRolloutSaving(false);
    }
  };

  const renderRuntimeCard = (card: EditableImageCard) => (
    <div key={card.local_id} className="rounded-lg border border-slate-200 bg-white p-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h3 className="text-base font-semibold text-gray-900">{card.display_name}</h3>
          <p className="mt-1 text-xs font-medium uppercase tracking-[0.14em] text-gray-500">
            {card.group === 'lite'
              ? t('systemSettingsPage.gatewayMode')
              : t('systemSettingsPage.desktopMode')}
          </p>
        </div>
        {!card.isBase && (
          <span className="inline-flex w-fit rounded-full bg-slate-100 px-2.5 py-1 text-xs font-medium text-slate-600">
            {t('systemSettingsPage.customProBadge')}
          </span>
        )}
      </div>

      {!card.isBase && (
        <div className="mt-4">
          <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.cardTitle')}</label>
          <input
            type="text"
            value={card.display_name}
            onChange={(event) => updateCard(card.local_id, { display_name: event.target.value })}
            className="app-input mt-1 block w-full"
          />
        </div>
      )}

      {card.runtime_type === 'desktop' && canSelectRuntimeVariant(card.instance_type) && (
        <div className="mt-4">
          <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.runtimeVariant')}</label>
          <select
            value={card.runtime_variant ?? 'windows'}
            onChange={(event) => updateRuntimeVariant(card, event.target.value as RuntimeVariant)}
            className="app-input mt-1 block w-full"
          >
            <option value="linux">{t('systemSettingsPage.linuxVariant')}</option>
            <option value="windows">{t('systemSettingsPage.windowsVariant')}</option>
          </select>
        </div>
      )}

      <div className="mt-4">
        <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.imageAddress')}</label>
        <input
          type="text"
          value={card.image}
          onChange={(event) => updateCard(card.local_id, { image: event.target.value })}
          placeholder={card.default_image}
          className="app-input mt-1 block w-full"
        />
      </div>

      <p className="mt-3 break-all text-xs text-gray-500">
        {t('systemSettingsPage.defaultImage')}: <span className="font-mono">{card.default_image ?? PRO_CUSTOM_DEFAULT_IMAGE}</span>
      </p>

      {card.error && (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
          {card.error}
        </div>
      )}

      <div className="mt-4 flex items-center justify-end gap-3">
        {!card.isBase && (
          <button
            type="button"
            onClick={() => void deleteCard(card)}
            disabled={card.saving}
            className="app-button-secondary inline-flex items-center gap-2 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Trash2 className="h-4 w-4" />
            {t('common.delete')}
          </button>
        )}
        <button
          type="button"
          onClick={() => void saveCard(card)}
          disabled={card.saving}
          className="app-button-primary inline-flex items-center gap-2 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Save className="h-4 w-4" />
          {card.saving ? t('modelManagementPage.saving') : t('common.save')}
        </button>
      </div>
    </div>
  );

  const renderEnterpriseStatus = () => {
    const status = enterpriseTestStatus ?? enterpriseConfig?.status;
    if (enterpriseLoading) {
      return <span className="text-sm text-gray-500">{t('common.loading')}</span>;
    }
    if (!status?.enabled) {
      return <span className="rounded-full bg-slate-100 px-2.5 py-1 text-xs font-medium text-slate-600">{t('systemSettingsPage.enterpriseDisabled')}</span>;
    }
    if (enterpriseStatusFailed(status)) {
      return <span className="rounded-full bg-red-100 px-2.5 py-1 text-xs font-medium text-red-700">{t('systemSettingsPage.enterpriseUnhealthy')}</span>;
    }
    if (status.warnings?.length) {
      return <span className="rounded-full bg-amber-100 px-2.5 py-1 text-xs font-medium text-amber-700">{t('systemSettingsPage.enterpriseWarning')}</span>;
    }
    return <span className="rounded-full bg-emerald-100 px-2.5 py-1 text-xs font-medium text-emerald-700">{t('systemSettingsPage.enterpriseHealthy')}</span>;
  };

  const bindPasswordConfigured = Boolean(enterpriseConfig?.bind_password_configured);
  const showBindPasswordMask = bindPasswordConfigured && !bindPasswordEditing && !clearBindPassword;
  const bindPasswordPlaceholder = bindPasswordConfigured
    ? t('systemSettingsPage.ldapPasswordReplacementPlaceholder')
    : t('systemSettingsPage.ldapPasswordPlaceholder');

  return (
    <AdminLayout title={t('admin.systemSettings')}>
      <div className="space-y-6">
        <PasswordSettingsSection />

        <section className="app-panel p-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h2 className="text-xl font-semibold text-gray-900">{t('systemSettingsPage.enterpriseTitle')}</h2>
              <p className="mt-1 text-sm text-gray-500">{t('systemSettingsPage.enterpriseLoginAliasHelp')}</p>
            </div>
            {renderEnterpriseStatus()}
          </div>

          {enterpriseError && (
            <div className="mt-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
              {enterpriseError}
            </div>
          )}
          {ldapForm.skip_tls_verify && (
            <div className="mt-4 flex gap-2 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t('systemSettingsPage.enterpriseSkipTLSWarning')}</span>
            </div>
          )}
          {(enterpriseTestStatus?.error || enterpriseTestStatus?.warnings?.length) && (
            <div className="mt-4 rounded-md border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">
              {enterpriseTestStatus.error
                ? localizeEnterpriseAuthIssue(enterpriseTestStatus.error, t)
                : localizeEnterpriseAuthIssues(enterpriseTestStatus.warnings, t).join(', ')}
            </div>
          )}

          {enterpriseLoading ? (
            <div className="mt-6 text-sm text-gray-500">{t('common.loading')}</div>
          ) : (
            <div className="mt-6 space-y-5">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <label className="flex items-center justify-between rounded-md border border-slate-200 px-4 py-3">
                  <span className="text-sm font-medium text-gray-700">{t('systemSettingsPage.enterpriseEnabled')}</span>
                  <input type="checkbox" checked={ldapEnabled} onChange={(event) => setLdapEnabled(event.target.checked)} className="h-4 w-4" />
                </label>
                <label className="flex items-center justify-between rounded-md border border-slate-200 px-4 py-3">
                  <span className="text-sm font-medium text-gray-700">{t('systemSettingsPage.enterpriseSyncRole')}</span>
                  <input type="checkbox" checked={syncRole} onChange={(event) => setSyncRole(event.target.checked)} className="h-4 w-4" />
                </label>
              </div>

              <div className="grid grid-cols-1 gap-4 lg:grid-cols-4">
                <div className="lg:col-span-3">
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapHost')}</label>
                  <input className="app-input mt-1 block w-full" value={ldapForm.host} onChange={(event) => updateLDAPForm({ host: event.target.value })} placeholder={LDAP_PLACEHOLDERS.host} />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapPort')}</label>
                  <input type="number" min={1} className="app-input mt-1 block w-full" value={ldapForm.port} onChange={(event) => updateLDAPForm({ port: Number(event.target.value) || 389 })} />
                </div>
                <label className="flex items-center gap-2 text-sm text-gray-700">
                  <input type="checkbox" checked={ldapForm.use_tls} onChange={(event) => updateLDAPForm({ use_tls: event.target.checked, start_tls: event.target.checked ? false : ldapForm.start_tls })} />
                  {t('systemSettingsPage.ldapUseTLS')}
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-700">
                  <input type="checkbox" checked={ldapForm.start_tls} onChange={(event) => updateLDAPForm({ start_tls: event.target.checked, use_tls: event.target.checked ? false : ldapForm.use_tls })} />
                  {t('systemSettingsPage.ldapStartTLS')}
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-700">
                  <input type="checkbox" checked={ldapForm.skip_tls_verify} onChange={(event) => updateLDAPForm({ skip_tls_verify: event.target.checked })} />
                  {t('systemSettingsPage.ldapSkipTLS')}
                </label>
                <div className="lg:col-span-2">
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapTLSCAFile')}</label>
                  <input className="app-input mt-1 block w-full" value={ldapForm.tls_ca_file} onChange={(event) => updateLDAPForm({ tls_ca_file: event.target.value })} placeholder={LDAP_PLACEHOLDERS.tlsCAFile} />
                </div>
                <div className="lg:col-span-2">
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapTLSServerName')}</label>
                  <input className="app-input mt-1 block w-full" value={ldapForm.tls_server_name} onChange={(event) => updateLDAPForm({ tls_server_name: event.target.value })} placeholder={LDAP_PLACEHOLDERS.tlsServerName} />
                </div>
              </div>

              <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
                <div>
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapBindDN')}</label>
                  <input className="app-input mt-1 block w-full" value={ldapForm.bind_dn} onChange={(event) => updateLDAPForm({ bind_dn: event.target.value })} placeholder={LDAP_PLACEHOLDERS.bindDN} />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">
                    {t('systemSettingsPage.ldapBindPassword')}
                    {bindPasswordConfigured ? <span className="ml-2 text-xs font-normal text-gray-500">{t('systemSettingsPage.ldapPasswordConfigured')}</span> : null}
                  </label>
                  {showBindPasswordMask ? (
                    <div className="mt-1 flex gap-2">
                      <input
                        type="password"
                        className="app-input block min-w-0 flex-1 bg-slate-50 text-slate-500"
                        value={t('systemSettingsPage.ldapSavedPasswordMask')}
                        readOnly
                        onFocus={() => setBindPasswordEditing(true)}
                      />
                      <button
                        type="button"
                        onClick={() => setBindPasswordEditing(true)}
                        className="app-button-secondary inline-flex shrink-0 items-center gap-2"
                      >
                        <Pencil className="h-4 w-4" />
                        {t('systemSettingsPage.ldapChangePassword')}
                      </button>
                    </div>
                  ) : (
                    <input
                      type="password"
                      className="app-input mt-1 block w-full"
                      value={bindPassword}
                      autoFocus={bindPasswordEditing && !clearBindPassword}
                      disabled={clearBindPassword}
                      onChange={(event) => setBindPassword(event.target.value)}
                      placeholder={bindPasswordPlaceholder}
                    />
                  )}
                  <label className="mt-2 flex items-center gap-2 text-xs text-gray-600">
                    <input type="checkbox" checked={clearBindPassword} onChange={(event) => handleClearBindPasswordChange(event.target.checked)} />
                    {t('systemSettingsPage.ldapClearPassword')}
                  </label>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapBaseDN')}</label>
                  <input className="app-input mt-1 block w-full" value={ldapForm.base_dn} onChange={(event) => updateLDAPForm({ base_dn: event.target.value })} placeholder={LDAP_PLACEHOLDERS.baseDN} />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapGroupBaseDN')}</label>
                  <input className="app-input mt-1 block w-full" value={ldapForm.group_base_dn} onChange={(event) => updateLDAPForm({ group_base_dn: event.target.value })} placeholder={LDAP_PLACEHOLDERS.groupBaseDN} />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapUserFilter')}</label>
                  <input className="app-input mt-1 block w-full font-mono" value={ldapForm.user_filter} onChange={(event) => updateLDAPForm({ user_filter: event.target.value })} placeholder={LDAP_PLACEHOLDERS.userFilter} />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapGroupFilter')}</label>
                  <input className="app-input mt-1 block w-full font-mono" value={ldapForm.group_filter} onChange={(event) => updateLDAPForm({ group_filter: event.target.value })} placeholder={LDAP_PLACEHOLDERS.groupFilter} />
                </div>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <div>
                    <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapUsernameAttribute')}</label>
                    <input className="app-input mt-1 block w-full" value={ldapForm.username_attribute} onChange={(event) => updateLDAPForm({ username_attribute: event.target.value })} />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapEmailAttribute')}</label>
                    <input className="app-input mt-1 block w-full" value={ldapForm.email_attribute} onChange={(event) => updateLDAPForm({ email_attribute: event.target.value })} />
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapDefaultRole')}</label>
                  <select className="app-input mt-1 block w-full" value={ldapForm.default_role} onChange={(event) => updateLDAPForm({ default_role: event.target.value })}>
                    <option value="user">{t('systemSettingsPage.ldapRoleUser')}</option>
                    <option value="admin">{t('systemSettingsPage.ldapRoleAdmin')}</option>
                  </select>
                </div>
                <div className="lg:col-span-2">
                  <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.ldapAdminGroups')}</label>
                  <textarea className="app-input mt-1 block min-h-24 w-full font-mono" value={adminGroupDNsText} onChange={(event) => setAdminGroupDNsText(event.target.value)} placeholder={LDAP_PLACEHOLDERS.adminGroups} />
                </div>
              </div>

              <div className="flex flex-col gap-3 border-t border-slate-200 pt-4 sm:flex-row sm:items-center sm:justify-end">
                <div className="flex flex-wrap gap-3">
                  <button type="button" onClick={() => void handleTestEnterpriseConfig()} disabled={enterpriseTesting || enterpriseSaving} className="app-button-secondary inline-flex items-center gap-2 disabled:cursor-not-allowed disabled:opacity-50">
                    <ShieldCheck className="h-4 w-4" />
                    {enterpriseTesting ? t('systemSettingsPage.enterpriseTesting') : t('systemSettingsPage.enterpriseTest')}
                  </button>
                  <button type="button" onClick={() => void handleSaveEnterpriseConfig()} disabled={enterpriseTesting || enterpriseSaving} className="app-button-primary inline-flex items-center gap-2 disabled:cursor-not-allowed disabled:opacity-50">
                    <KeyRound className="h-4 w-4" />
                    {enterpriseSaving ? t('modelManagementPage.saving') : t('systemSettingsPage.enterpriseSave')}
                  </button>
                </div>
              </div>
            </div>
          )}
        </section>

        <section className="app-panel p-6">
          <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
            <div className="flex flex-col gap-1">
              <h2 className="text-xl font-semibold text-gray-900">{t('systemSettingsPage.liteRolloutTitle')}</h2>
              <p className="text-sm text-gray-500">{t('systemSettingsPage.liteRolloutSubtitle')}</p>
            </div>
          </div>
          <div className="mt-5 grid grid-cols-1 gap-4 lg:grid-cols-[minmax(180px,240px)_1fr] xl:grid-cols-[minmax(180px,240px)_minmax(260px,1fr)_minmax(320px,1.4fr)_120px_140px_auto] xl:items-end">
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.liteRuntime')}</label>
              <select
                value={rolloutRuntimeType}
                onChange={(event) => handleRolloutRuntimeTypeChange(event.target.value as RuntimeType)}
                className="app-input mt-1 block w-full"
              >
                {LITE_RUNTIME_CARDS.map((option) => (
                  <option key={option.instance_type} value={option.instance_type}>
                    {option.display_name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.currentGatewayImage')}</label>
              <div className="mt-1 min-h-10 break-all rounded-md border border-slate-200 bg-slate-50 px-3 py-2 font-mono text-sm text-slate-700">
                {rolloutCurrentLoading ? t('common.loading') : rolloutCurrentImage || 'N/A'}
              </div>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.targetGatewayImage')}</label>
              <input
                type="text"
                value={rolloutImage}
                onChange={(event) => { setRolloutImage(event.target.value); setRolloutPreflight(null); setRolloutError(null); }}
                className="app-input mt-1 block w-full"
                placeholder={rolloutRuntimeType === 'openclaw'
				  ? 'registry/repository:tag 或 registry/repository@sha256:...'
                  : rolloutCard?.default_image}
              />
              {rolloutRuntimeType === 'openclaw' && (
                <p className="mt-1 text-xs text-slate-500">
                  {t('systemSettingsPage.rolloutImmutableTargetHelp')}
                </p>
              )}
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.rolloutBatch')}</label>
              <input
                type="number"
                min={1}
                value={rolloutBatchSize}
                onChange={(event) => { setRolloutBatchSize(Number(event.target.value) || 1); setRolloutPreflight(null); }}
                className="app-input mt-1 block w-full"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">{t('systemSettingsPage.rolloutUnavailable')}</label>
              <input
                type="number"
				min={rolloutPreflight?.strategy === 'openclaw_8plus_data_safe' ? 0 : 1}
                value={rolloutMaxUnavailable}
                onChange={(event) => { setRolloutMaxUnavailable(Number(event.target.value) || 0); setRolloutPreflight(null); }}
				disabled={rolloutPreflight?.strategy === 'openclaw_8plus_data_safe'}
                className="app-input mt-1 block w-full"
              />
            </div>
            <button
              type="button"
              onClick={() => void startRollout()}
				disabled={rolloutSaving || activeRolloutId !== null}
              className="app-button-primary inline-flex items-center justify-center gap-2 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Rocket className="h-4 w-4" />
			  {activeRolloutId !== null
				? `升级执行中（#${activeRolloutId}）`
				: rolloutSaving
                ? t('systemSettingsPage.rolloutStarting')
				: rolloutRuntimeType === 'openclaw' && rolloutPreflight?.strategy === 'openclaw_8plus_data_safe' && rolloutPreflight.passed
                  ? '确认执行（自动回退）'
                  : rolloutRuntimeType === 'openclaw'
                    ? '执行升级预检'
                    : t('systemSettingsPage.startRollout')}
            </button>
          </div>
          {rolloutError && (
            <div className="mt-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
              {rolloutError}
            </div>
          )}
		  {rolloutDetails && (
			<div className="mt-4 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-900">
			  <div className="font-medium">
				升级 #{rolloutDetails.rollout.id}：{rolloutDetails.rollout.status} / {rolloutDetails.rollout.phase}
			  </div>
			  <div className="mt-1">
				目标：<span className="font-mono break-all">{rolloutDetails.rollout.target_image_ref}</span>
			  </div>
			  <div className="mt-1">
				实例 {rolloutDetails.items.length} 个；已验证 {rolloutDetails.items.filter((item) => ['gateway_verified', 'verified'].includes(item.state)).length} 个；已恢复 {rolloutDetails.items.filter((item) => ['restored', 'restart_ready'].includes(item.state)).length} 个。
			  </div>
			  {rolloutDetails.rollout.rollback_status && <div className="mt-1">自动回退：{rolloutDetails.rollout.rollback_status}</div>}
			  {(rolloutDetails.rollout.error_message || rolloutDetails.rollout.rollback_error) && (
				<div className="mt-2 text-red-700">{rolloutDetails.rollout.error_message || rolloutDetails.rollout.rollback_error}</div>
			  )}
			</div>
		  )}
          {rolloutPreflight && (
            <div className={`mt-4 rounded-md border px-4 py-3 text-sm ${rolloutPreflight.passed ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-amber-200 bg-amber-50 text-amber-900'}`}>
              <div className="font-medium">
                {rolloutPreflight.passed ? '预检通过，可确认执行' : '预检未通过，未修改 Runtime 或用户数据'}
              </div>
              <div className="mt-1">
                实例 {rolloutPreflight.instance_count} 个，Team {rolloutPreflight.team_count} 个，OpenClaw Team 成员 {rolloutPreflight.openclaw_team_member_count} 个，Hermes 成员 {rolloutPreflight.hermes_team_member_count} 个（Hermes 不升级）。
              </div>
              {rolloutPreflight.warnings.length > 0 && <div className="mt-2">提示：{rolloutPreflight.warnings.join('；')}</div>}
              {rolloutPreflight.blockers.length > 0 && <div className="mt-2">阻断：{rolloutPreflight.blockers.join('；')}</div>}
			  {rolloutPreflight.rollout?.plan_fingerprint && <div className="mt-2 font-mono text-xs break-all">审计指纹：{rolloutPreflight.rollout.plan_fingerprint}</div>}
            </div>
          )}
        </section>

        <section className="app-panel p-6">
          <div>
            <h2 className="text-xl font-semibold text-gray-900">{t('systemSettingsPage.liteRuntimeTitle')}</h2>
            <p className="mt-1 text-sm text-gray-500">{t('systemSettingsPage.liteRuntimeSubtitle')}</p>
          </div>
          {pageError && (
            <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
              {pageError}
            </div>
          )}
          {loading ? (
            <div className="mt-6 text-sm text-gray-500">{t('common.loading')}</div>
          ) : (
            <div className="mt-6 grid grid-cols-1 gap-4 xl:grid-cols-2">
              {liteCards.map(renderRuntimeCard)}
            </div>
          )}
        </section>

        <section className="app-panel p-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="text-xl font-semibold text-gray-900">{t('systemSettingsPage.proRuntimeTitle')}</h2>
              <p className="mt-1 text-sm text-gray-500">{t('systemSettingsPage.proRuntimeSubtitle')}</p>
            </div>
            <button
              type="button"
              onClick={addProCustomCard}
              className="app-button-primary inline-flex items-center gap-2"
            >
              <Plus className="h-4 w-4" />
              {t('systemSettingsPage.addProCustomCard')}
            </button>
          </div>
          {loading ? (
            <div className="mt-6 text-sm text-gray-500">{t('common.loading')}</div>
          ) : (
            <div className="mt-6 grid grid-cols-1 gap-4 xl:grid-cols-2">
              {[...proBaseCards, ...proCustomCards].map(renderRuntimeCard)}
            </div>
          )}
        </section>
      </div>
    </AdminLayout>
  );
};

export default SystemSettingsPage;
