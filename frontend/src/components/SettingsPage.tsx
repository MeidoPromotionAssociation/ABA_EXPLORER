// frontend/src/components/SettingsPage.tsx
import React, {useState} from "react";
import {
    Button,
    Card,
    Col,
    ColorPicker,
    Dropdown,
    List,
    MenuProps,
    Row,
    Segmented,
    Space,
    Switch,
    Tooltip,
    Typography,
} from "antd";
import {useTranslation} from "react-i18next";
import {
    CloseCircleOutlined,
    DownOutlined,
    GithubOutlined,
    QuestionCircleOutlined,
    SyncOutlined,
    TranslationOutlined,
} from "@ant-design/icons";
import {Browser} from "@wailsio/runtime";
import {DefaultThemeColor, ThemeMode, useThemeColor, useThemeMode} from "../hooks/themeSwitch";
import {checkForUpdatesWithMessage} from "../utils/CheckUpdate";
import {
    AppVersion,
    GitHubRepoUrl,
    MeidoSerializationUrl,
    ModEditorUrl,
    SettingCheckUpdateKey,
} from "../utils/consts";
import {NewVersionAvailableKey} from "../utils/LocalStorageKeys";
import {resolveUiLanguage} from "../utils/i18n";

const SettingsPage: React.FC = () => {
    const {t, i18n} = useTranslation();
    // 检测到的语言可能是 en-GB 这类标签，收敛后语言菜单才能显示正确的当前项
    const [language, setLanguage] = useState(resolveUiLanguage);
    const [themeMode, setThemeMode] = useThemeMode();
    const [themeColor, setThemeColor] = useThemeColor();

    const [checkUpdates, setCheckUpdates] = useState(() => {
        const saved = localStorage.getItem(SettingCheckUpdateKey);
        return saved ? JSON.parse(saved) : true;
    });

    const handleUpdateCheck = (checked: boolean) => {
        setCheckUpdates(checked);
        localStorage.setItem(SettingCheckUpdateKey, JSON.stringify(checked));
    };

    const handleDismissUpdate = () => {
        localStorage.setItem(NewVersionAvailableKey, 'false');
    };

    const handleLanguageChange: MenuProps['onClick'] = (e) => {
        i18n.changeLanguage(e.key).then(() => {
        });
        setLanguage(e.key);
    };

    const languageMenu: MenuProps = {
        items: [
            {label: '简体中文 (Simplified Chinese)', key: "zh-CN"},
            {label: 'English (American English)', key: "en-US"},
            {label: '日本語 (Japanese)', key: "ja-JP"},
            {label: '韓國語 (Korean)', key: "ko-KR"},
        ],
        onClick: handleLanguageChange,
    };

    const languageLabel = (value: string) => {
        switch (value) {
            case 'zh-CN':
                return '简体中文 (Simplified Chinese)';
            case 'en-US':
                return 'English (American English)';
            case 'ja-JP':
                return '日本語 (Japanese)';
            case 'ko-KR':
                return '韓國語 (Korean)';
            default:
                return '简体中文 (Simplified Chinese)';
        }
    };

    return (
        <div style={{flex: 1, minWidth: 0, height: "100%", overflow: "auto", padding: 8}}>
            <Row gutter={[16, 16]}>
                <Col span={24} style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center'}}>
                    <Typography.Title level={4} style={{margin: 0}}>{t('NavBar.settings')}</Typography.Title>
                </Col>

                {/* 通用设置区域 */}
                <Col xs={24} lg={12}>
                    <Card
                        title={<Typography.Title level={5}>{t('SettingsPage.general_settings')}</Typography.Title>}
                        style={{borderRadius: 8}}
                    >
                        <List
                            split={false}
                            dataSource={[
                                {
                                    title: t('Common.choose_language'),
                                    type: 'language',
                                    value: language
                                },
                                {
                                    title: t('SettingsPage.theme_mode'),
                                    tooltip: t('SettingsPage.theme_mode_tip'),
                                    type: 'theme'
                                },
                                {
                                    title: t('SettingsPage.theme_color'),
                                    tooltip: t('SettingsPage.theme_color_tip'),
                                    type: 'themeColor'
                                }
                            ]}
                            renderItem={(item: any) => (
                                <List.Item
                                    style={{padding: '16px 0'}}
                                    actions={[
                                        item.type === 'theme' ? (
                                            <Segmented
                                                key="theme"
                                                value={themeMode}
                                                onChange={(value) => setThemeMode(value as ThemeMode)}
                                                options={[
                                                    {label: t('SettingsPage.theme_system'), value: 'system'},
                                                    {label: t('SettingsPage.theme_light'), value: 'light'},
                                                    {label: t('SettingsPage.theme_dark'), value: 'dark'},
                                                ]}
                                            />
                                        ) : item.type === 'themeColor' ? (
                                            <Space key="themeColor">
                                                {themeColor ? (
                                                    <Button size="small" onClick={() => setThemeColor(null)}>
                                                        {t('SettingsPage.theme_color_reset')}
                                                    </Button>
                                                ) : null}
                                                <ColorPicker
                                                    value={themeColor ?? DefaultThemeColor}
                                                    showText
                                                    disabledAlpha
                                                    presets={[{
                                                        label: t('SettingsPage.theme_color_presets'),
                                                        colors: [...new Set([
                                                            DefaultThemeColor, '#1890ff', '#f5222d', '#fa541c', '#fa8c16',
                                                            '#faad14', '#a0d911', '#52c41a', '#13c2c2',
                                                            '#2f54eb', '#722ed1', '#eb2f96', '#8c8c8c', '#389E0D',
                                                        ])],
                                                    }]}
                                                    onChangeComplete={(color) => setThemeColor(color.toHexString())}
                                                />
                                            </Space>
                                        ) : (
                                            <Dropdown key="language" menu={languageMenu} placement="bottomRight">
                                                <Button>
                                                    {languageLabel(item.value)} <TranslationOutlined/>
                                                    <DownOutlined style={{marginLeft: 8}}/>
                                                </Button>
                                            </Dropdown>
                                        )
                                    ]}
                                >
                                    <Space>
                                        <span>{item.title}</span>
                                        {item.tooltip ? (
                                            <Tooltip title={item.tooltip}>
                                                <QuestionCircleOutlined style={{color: '#aaa'}}/>
                                            </Tooltip>
                                        ) : null}
                                    </Space>
                                </List.Item>
                            )}
                        />
                    </Card>
                </Col>
                {/* 更新设置区域 */}
                <Col xs={24} lg={12}>
                    <Card
                        title={<Typography.Title level={5}>{t('SettingsPage.update_settings')}</Typography.Title>}
                        style={{borderRadius: 8}}
                    >
                        <List
                            split={false}
                            dataSource={[
                                {
                                    title: t('SettingsPage.is_check_update'),
                                    tooltip: t('SettingsPage.is_check_update_tip'),
                                    checked: checkUpdates,
                                    onChange: handleUpdateCheck,
                                    type: 'switch'
                                },
                                {
                                    title: t('SettingsPage.dismiss_update_note'),
                                    tooltip: t('SettingsPage.dismiss_update_note_tip'),
                                    type: 'button',
                                    icon: <CloseCircleOutlined/>,
                                    onClick: handleDismissUpdate
                                },
                                {
                                    title: t('SettingsPage.check_update_now'),
                                    tooltip: '',
                                    type: 'button',
                                    icon: <SyncOutlined/>,
                                    onClick: () => checkForUpdatesWithMessage()
                                },
                            ]}
                            renderItem={(item: any) => (
                                <List.Item
                                    style={{padding: '16px 0'}}
                                    actions={[
                                        item.type === 'switch' ? (
                                            <Switch
                                                key="switch"
                                                checked={item.checked}
                                                onChange={item.onChange}
                                            />
                                        ) : (
                                            <Button
                                                key="button"
                                                icon={item.icon}
                                                onClick={item.onClick}
                                                style={{borderRadius: 4}}
                                            >
                                                {item.title}
                                            </Button>
                                        )
                                    ]}
                                >
                                    <Space>
                                        <span>{item.title}</span>
                                        {item.tooltip ? (
                                            <Tooltip title={item.tooltip}>
                                                <QuestionCircleOutlined style={{color: '#aaa'}}/>
                                            </Tooltip>
                                        ) : null}
                                    </Space>
                                </List.Item>
                            )}
                        />
                    </Card>
                </Col>

                {/* 关于区域 */}
                <Col span={24}>
                    <Card
                        title={<Typography.Title level={5}>{t('SettingsPage.about')}</Typography.Title>}
                        style={{borderRadius: 8}}
                    >
                        <List
                            split={false}
                            dataSource={[
                                {
                                    title: t('SettingsPage.version'),
                                    type: 'text',
                                    value: AppVersion
                                },
                                {
                                    title: t('SettingsPage.engine'),
                                    tooltip: t('SettingsPage.engine_tip'),
                                    type: 'text',
                                    value: t('SettingsPage.engine_desc')
                                },
                                {
                                    title: t('SettingsPage.links'),
                                    type: 'links'
                                },
                            ]}
                            renderItem={(item: any) => (
                                <List.Item
                                    style={{padding: '16px 0'}}
                                    actions={[
                                        item.type === 'links' ? (
                                            <Space key="links" wrap>
                                                <Button size="small" icon={<GithubOutlined/>}
                                                        onClick={() => Browser.OpenURL(GitHubRepoUrl)}>
                                                    ABA_EXPLORER
                                                </Button>
                                                <Button size="small" icon={<GithubOutlined/>}
                                                        onClick={() => Browser.OpenURL(MeidoSerializationUrl)}>
                                                    MeidoSerialization
                                                </Button>
                                                <Button size="small" icon={<GithubOutlined/>}
                                                        onClick={() => Browser.OpenURL(ModEditorUrl)}>
                                                    KCES_MOD_EDITOR
                                                </Button>
                                            </Space>
                                        ) : (
                                            <Typography.Text key="text" type="secondary">{item.value}</Typography.Text>
                                        )
                                    ]}
                                >
                                    <Space>
                                        <span>{item.title}</span>
                                        {item.tooltip ? (
                                            <Tooltip title={item.tooltip}>
                                                <QuestionCircleOutlined style={{color: '#aaa'}}/>
                                            </Tooltip>
                                        ) : null}
                                    </Space>
                                </List.Item>
                            )}
                        />
                    </Card>
                </Col>
            </Row>
        </div>
    );
};

export default SettingsPage;
