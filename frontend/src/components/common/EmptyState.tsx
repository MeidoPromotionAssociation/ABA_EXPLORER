import React from "react";
import {Button, Empty, Typography} from "antd";
import {InboxOutlined} from "@ant-design/icons";

const {Paragraph} = Typography;

interface EmptyStateProps {
    description: string;
    hint?: string;
    actionLabel?: string;
    onAction?: () => void;
}

/**
 * EmptyState 未打开文件时的占位
 * 各浏览页共用，把"从哪里打开"这件事说清楚，包括支持拖放
 */
const EmptyState: React.FC<EmptyStateProps> = ({description, hint, actionLabel, onAction}) => (
    <div
        style={{
            flex: 1,
            minWidth: 0,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
        }}
    >
        <Empty
            image={<InboxOutlined style={{fontSize: 56, opacity: 0.45}}/>}
            description={
                <div>
                    <Paragraph style={{marginBottom: hint ? 4 : 0}}>{description}</Paragraph>
                    {hint && (
                        <Paragraph type="secondary" style={{fontSize: 12, marginBottom: 0}}>
                            {hint}
                        </Paragraph>
                    )}
                </div>
            }
        >
            {actionLabel && onAction && (
                <Button type="primary" onClick={onAction}>
                    {actionLabel}
                </Button>
            )}
        </Empty>
    </div>
);

export default EmptyState;
