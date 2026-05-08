import React, {useCallback, useEffect, useState} from 'react';
import {
    Alert,
    Box,
    Chip,
    CircularProgress,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    Tooltip,
    Typography,
} from '@mui/material';
import CameraAltOutlinedIcon from '@mui/icons-material/CameraAltOutlined';
import CloseIcon from '@mui/icons-material/Close';
import {useTranslation} from 'react-i18next';
import UnifiedCard from '@/components/UnifiedCard';
import ModelSelectDialog, {type ProviderSelectTabOption} from '@/components/ModelSelectDialog';
import type {Provider} from '@/types/provider';
import type {VisionProxyConfig} from '@/components/RoutingGraphTypes';
import api from '@/services/api';

interface VisionProxyCardProps {
    scenario: string;
    providers: Provider[];
}

const EMPTY_CONFIG: VisionProxyConfig = {
    enabled: false,
    provider_id: '',
    model: '',
    timeout_ms: 15000,
};

const VisionProxyCard: React.FC<VisionProxyCardProps> = ({scenario, providers}) => {
    const {t} = useTranslation();
    const [visionProxy, setVisionProxy] = useState<VisionProxyConfig>(EMPTY_CONFIG);
    const [fullConfig, setFullConfig] = useState<any>(null);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [selectOpen, setSelectOpen] = useState(false);
    const [providerMissing, setProviderMissing] = useState(false);

    useEffect(() => {
        if (!scenario) return;
        setLoading(true);
        api.getScenarioConfig(scenario)
            .then((result: any) => {
                if (result?.success && result.data) {
                    setFullConfig(result.data);
                    const vp: VisionProxyConfig = result.data.vision_proxy ?? {};
                    setVisionProxy({...EMPTY_CONFIG, ...vp});
                }
            })
            .finally(() => setLoading(false));
    }, [scenario]);

    // Check if the configured provider still exists
    useEffect(() => {
        if (!visionProxy.provider_id) {
            setProviderMissing(false);
            return;
        }
        const found = providers.some(p => p.uuid === visionProxy.provider_id);
        setProviderMissing(!found);
    }, [visionProxy.provider_id, providers]);

    const saveConfig = useCallback(async (newVp: VisionProxyConfig) => {
        setSaving(true);
        try {
            const config = {...fullConfig, scenario, vision_proxy: newVp};
            await api.setScenarioConfig(scenario, config);
            setFullConfig((prev: any) => ({...prev, vision_proxy: newVp}));
        } finally {
            setSaving(false);
        }
    }, [fullConfig, scenario]);

    const toggleEnabled = useCallback(async () => {
        const newVp = {...visionProxy, enabled: !visionProxy.enabled};
        setVisionProxy(newVp);
        await saveConfig(newVp);
    }, [visionProxy, saveConfig]);

    const handleModelSelected = useCallback(async (option: ProviderSelectTabOption) => {
        const newVp: VisionProxyConfig = {
            ...visionProxy,
            provider_id: option.provider.uuid,
            model: option.model,
        };
        setVisionProxy(newVp);
        setSelectOpen(false);
        await saveConfig(newVp);
    }, [visionProxy, saveConfig]);

    const selectedProvider = providers.find(p => p.uuid === visionProxy.provider_id);
    const isConfigured = Boolean(visionProxy.provider_id && visionProxy.model);
    const modelLabel = isConfigured
        ? `${selectedProvider?.name ?? visionProxy.provider_id} · ${visionProxy.model}`
        : t('visionProxy.selectModel');

    const toggleChip = (
        <Box sx={{display: 'flex', alignItems: 'center', gap: 1}}>
            {saving && <CircularProgress size={14}/>}
            <Chip
                size="small"
                icon={<CameraAltOutlinedIcon fontSize="small"/>}
                label={`${t('visionProxy.title')} · ${visionProxy.enabled ? t('common.on') : t('common.off')}`}
                onClick={toggleEnabled}
                color={visionProxy.enabled ? 'primary' : 'default'}
                variant={visionProxy.enabled ? 'filled' : 'outlined'}
                disabled={loading || saving}
            />
            <Tooltip title={t('visionProxy.selectModel')} arrow>
                <Chip
                    size="small"
                    label={modelLabel}
                    onClick={() => setSelectOpen(true)}
                    variant="outlined"
                    disabled={loading || saving}
                    sx={{maxWidth: 260, '.MuiChip-label': {overflow: 'hidden', textOverflow: 'ellipsis'}}}
                />
            </Tooltip>
        </Box>
    );

    return (
        <>
            <UnifiedCard size="full" title={t('visionProxy.title')} rightAction={toggleChip}>
                <Box sx={{px: 0.5, py: 0.5}}>
                    {loading ? (
                        <CircularProgress size={20}/>
                    ) : (
                        <Typography variant="body2" color="text.secondary">
                            {t('visionProxy.description')}
                        </Typography>
                    )}
                    {providerMissing && visionProxy.enabled && (
                        <Alert severity="warning" sx={{mt: 1}}>
                            {t('visionProxy.unavailableProvider')}
                        </Alert>
                    )}
                </Box>
            </UnifiedCard>

            <Dialog
                open={selectOpen}
                onClose={() => setSelectOpen(false)}
                maxWidth="md"
                fullWidth
                PaperProps={{sx: {height: '70vh'}}}
            >
                <DialogTitle sx={{display: 'flex', alignItems: 'center', justifyContent: 'space-between'}}>
                    <span>{t('visionProxy.selectModel')}</span>
                    <IconButton size="small" onClick={() => setSelectOpen(false)}>
                        <CloseIcon fontSize="small"/>
                    </IconButton>
                </DialogTitle>
                <DialogContent sx={{p: 0, overflow: 'hidden'}}>
                    <ModelSelectDialog
                        providers={providers}
                        selectedProvider={visionProxy.provider_id}
                        selectedModel={visionProxy.model}
                        onSelected={handleModelSelected}
                    />
                </DialogContent>
            </Dialog>
        </>
    );
};

export default VisionProxyCard;
