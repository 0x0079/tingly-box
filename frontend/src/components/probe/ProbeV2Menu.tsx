import React, { useState } from 'react';
import {
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText,
    Divider,
    Typography,
} from '@mui/material';
import {
    PlayArrow as DirectIcon,
    Stream as StreamingIcon,
    Build as ToolIcon,
} from '@mui/icons-material';
import { useTranslation } from 'react-i18next';
import { ProbeV2Dialog } from './ProbeV2Dialog';
import type { ProbeV2TestMode, ProbeV2TargetType } from '@/types/probe-v2.ts';

interface ProbeV2MenuProps {
    anchorEl: HTMLElement | null;
    open: boolean;
    onClose: () => void;
    targetType: ProbeV2TargetType;
    targetId: string;
    targetName: string;
    scenario?: string;
    model?: string;
    disabledReason?: string;
}

interface ProbeOption {
    mode: ProbeV2TestMode;
    labelKey: string;
    descriptionKey: string;
    icon: React.ReactNode;
}

const PROBE_OPTIONS: ProbeOption[] = [
    {
        mode: 'simple',
        labelKey: 'probe.menu.options.simple.label',
        icon: <DirectIcon fontSize="small" />,
        descriptionKey: 'probe.menu.options.simple.description',
    },
    {
        mode: 'streaming',
        labelKey: 'probe.menu.options.streaming.label',
        icon: <StreamingIcon fontSize="small" />,
        descriptionKey: 'probe.menu.options.streaming.description',
    },
    {
        mode: 'tool',
        labelKey: 'probe.menu.options.tool.label',
        icon: <ToolIcon fontSize="small" />,
        descriptionKey: 'probe.menu.options.tool.description',
    },
];

export const ProbeV2Menu: React.FC<ProbeV2MenuProps> = ({
    anchorEl,
    open,
    onClose,
    targetType,
    targetId,
    targetName,
    scenario,
    model,
    disabledReason,
}) => {
    const { t } = useTranslation();
    const [dialogOpen, setDialogOpen] = useState(false);
    const [selectedMode, setSelectedMode] = useState<ProbeV2TestMode>('simple');

    const handleProbeClick = (mode: ProbeV2TestMode) => {
        if (disabledReason) {
            return;
        }
        setSelectedMode(mode);
        setDialogOpen(true);
        onClose();
    };

    const handleDialogClose = () => {
        setDialogOpen(false);
    };

    const getTargetTypeLabel = () => {
        switch (targetType) {
            case 'provider':
                return t('probe.menu.target.provider');
            case 'rule':
                return t('probe.menu.target.rule');
            default:
                return t('probe.menu.target.default');
        }
    };

    return (
        <>
            <Menu
                anchorEl={anchorEl}
                open={open}
                onClose={onClose}
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
                PaperProps={{
                    sx: { minWidth: 250 }
                }}
            >
                <MenuItem disabled sx={{ opacity: 1 }}>
                    <Typography variant="subtitle2" color="text.secondary">
                        {t('probe.menu.title', { target: getTargetTypeLabel() })}
                    </Typography>
                </MenuItem>
                <Divider />
                {disabledReason ? (
                    <MenuItem disabled>
                        <ListItemText
                            primary={disabledReason}
                            primaryTypographyProps={{
                                variant: 'body2',
                                color: 'text.secondary',
                            }}
                        />
                    </MenuItem>
                ) : PROBE_OPTIONS.map((option) => (
                    <MenuItem
                        key={option.mode}
                        onClick={() => handleProbeClick(option.mode)}
                    >
                        <ListItemIcon>
                            {option.icon}
                        </ListItemIcon>
                        <ListItemText
                            primary={t(option.labelKey)}
                            secondary={t(option.descriptionKey)}
                            secondaryTypographyProps={{
                                variant: 'caption',
                                sx: { fontSize: '0.75rem' }
                            }}
                        />
                    </MenuItem>
                ))}
            </Menu>

            <ProbeV2Dialog
                open={dialogOpen}
                onClose={handleDialogClose}
                targetType={targetType}
                targetId={targetId}
                targetName={targetName}
                scenario={scenario}
                model={model}
                testMode={selectedMode}
            />
        </>
    );
};

export default ProbeV2Menu;
