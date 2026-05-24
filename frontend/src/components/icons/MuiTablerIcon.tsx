import React from 'react';
import { SvgIcon, type SvgIconProps } from '@mui/material';
import type { Icon as TablerIconComponent } from '@tabler/icons-react';

type MuiTablerIconProps = Omit<SvgIconProps, 'component'> & {
  icon: TablerIconComponent;
};

export function MuiTablerIcon({ icon, ...props }: MuiTablerIconProps) {
  return <SvgIcon component={icon} inheritViewBox {...props} />;
}
