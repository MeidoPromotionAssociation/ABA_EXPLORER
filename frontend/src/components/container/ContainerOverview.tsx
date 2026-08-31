import React, {useState} from "react";
import {Collapse, Descriptions, Space, Tag, Typography} from "antd";
import {useTranslation} from "react-i18next";
import type {AbaOverview} from "../../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal/models";
import {ContainerOverviewOpenKey} from "../../utils/LocalStorageKeys";
import {formatBytes, formatNumber, percent} from "../../utils/format";

const {Text} = Typography;

/**
 * ContainerOverview 容器头部信息面板
 * 默认折叠，把纵向空间让给对象表；折叠时标题栏仍显示种类、引擎版本、大小与对象数
 * 展开状态记在 localStorage 里跨会话保留
 */
const ContainerOverview: React.FC<{ overview: AbaOverview }> = ({overview}) => {
    const {t} = useTranslation();
    const [open, setOpen] = useState(() => localStorage.getItem(ContainerOverviewOpenKey) === "true");

    const summary = (
        <Space size={8} wrap>
            <Tag color="blue" style={{marginInlineEnd: 0}}>{overview.kind}</Tag>
            <Text type="secondary" style={{fontSize: 12}}>{overview.engineVersion || overview.signature}</Text>
            <Text type="secondary" style={{fontSize: 12}}>{formatBytes(overview.fileSize)}</Text>
            <Text type="secondary" style={{fontSize: 12}}>
                {t("ContainerPage.counts_value", {
                    blocks: formatNumber(overview.blocks.length),
                    directories: formatNumber(overview.directories.length),
                    assets: formatNumber(overview.assetCount),
                })}
            </Text>
        </Space>
    );

    return (
        <Collapse
            size="small"
            style={{marginBottom: 12, flex: "none"}}
            activeKey={open ? ["overview"] : []}
            onChange={(keys) => {
                const next = keys.length > 0;
                setOpen(next);
                localStorage.setItem(ContainerOverviewOpenKey, String(next));
            }}
            items={[{
                key: "overview",
                label: summary,
                children: (
                    <Descriptions
                        size="small"
                        column={{xs: 1, sm: 2, lg: 3, xxl: 4}}
                        items={[
                            {
                                key: "kind",
                                label: t("ContainerPage.kind"),
                                children: <Tag color="blue">{overview.kind}</Tag>,
                            },
                            {key: "signature", label: t("ContainerPage.signature"), children: overview.signature},
                            {
                                key: "formatVersion",
                                label: t("ContainerPage.format_version"),
                                children: overview.formatVersion,
                            },
                            {
                                key: "engineVersion",
                                label: t("ContainerPage.engine_version"),
                                children: overview.engineVersion || "-",
                            },
                            {
                                key: "generationVersion",
                                label: t("ContainerPage.generation_version"),
                                children: overview.generationVersion || "-",
                            },
                            {
                                key: "fileSize",
                                label: t("ContainerPage.file_size"),
                                children: formatBytes(overview.fileSize),
                            },
                            {
                                key: "compressedData",
                                label: t("ContainerPage.compressed_data"),
                                children: `${formatBytes(overview.compressedData)} (${percent(
                                    overview.compressedData,
                                    overview.fileSize
                                )})`,
                            },
                            {
                                key: "metadataCompression",
                                label: t("ContainerPage.metadata_compression"),
                                children: overview.metadataCompression,
                            },
                            {
                                key: "counts",
                                label: t("ContainerPage.counts"),
                                children: t("ContainerPage.counts_value", {
                                    blocks: formatNumber(overview.blocks.length),
                                    directories: formatNumber(overview.directories.length),
                                    assets: formatNumber(overview.assetCount),
                                }),
                            },
                            {
                                key: "hash",
                                label: t("ContainerPage.hash"),
                                children: <Text className="mono" copyable ellipsis>{overview.hash}</Text>,
                                span: 2,
                            },
                        ]}
                    />
                ),
            }]}
        />
    );
};

export default ContainerOverview;
