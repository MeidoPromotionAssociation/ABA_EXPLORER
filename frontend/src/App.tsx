// App.tsx
import React, {useEffect} from "react";
import {Route, Routes} from "react-router-dom";
import {App as AntdApp, ConfigProvider, Layout, theme} from "antd";
import type {Locale} from "antd/es/locale";
import zhCN from "antd/locale/zh_CN";
import enUS from "antd/locale/en_US";
import jaJP from "antd/locale/ja_JP";
import koKR from "antd/locale/ko_KR";
import {useTranslation} from "react-i18next";
import {Events} from "@wailsio/runtime";
import NavBar from "./components/NavBar";
import HomePage from "./components/HomePage";
import ContainerPage from "./components/ContainerPage";
import CtPage from "./components/CtPage";
import UnpackedPage from "./components/UnpackedPage";
import PackPage from "./components/PackPage";
import SettingsPage from "./components/SettingsPage";
import {DefaultThemeColor, useDarkMode, useThemeColor} from "./hooks/themeSwitch";
import useFileOpener from "./hooks/fileOpener";
import {bindMessage} from "./utils/feedback";
import {resolveUiLanguage} from "./utils/i18n";
import {App as AppService} from "../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";

const {Content} = Layout;

// antd 组件文案跟随界面语言，未覆盖的语言回落到简体中文
const AntdLocales: Record<string, Locale> = {
    "zh-CN": zhCN,
    "en-US": enUS,
    "ja-JP": jaJP,
    "ko-KR": koKR,
};

// MessageBinder 把组件树内（可消费主题上下文）的 message 实例绑定到全局桥
const MessageBinder: React.FC = () => {
    const {message} = AntdApp.useApp();
    useEffect(() => {
        bindMessage(message);
    }, [message]);
    return null;
};

const App: React.FC = () => {
    const isDarkMode = useDarkMode();
    const [themeColor] = useThemeColor();
    const {openPath, selectAndOpen} = useFileOpener();

    // 订阅语言变化，切换语言后重新解析 antd 的 locale
    // Subscribing to language changes re-resolves the antd locale after a switch
    useTranslation();

    // antd 的 locale 键必须是实际有翻译的四个语言码，webview 报 en-GB 这类标签时要先收敛
    // The antd locale key must be one of the four languages we ship, so tags such as en-GB are normalized first
    const antdLocale = AntdLocales[resolveUiLanguage()] ?? zhCN;

    // 通过文件关联启动时打开传入的文件
    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const path = await AppService.StartupFile();
                if (!cancelled && path) await openPath(path);
            } catch (error) {
                console.error("open startup file failed:", error);
            }
        })();
        return () => {
            cancelled = true;
        };
        // 只在挂载时执行一次，openPath 的身份变化不应重新打开启动文件
    }, []);

    // 用户拖放文件
    useEffect(() => {
        const off = Events.On("explorer:file-dropped", async (event: any) => {
            const data = event?.data;
            const path = Array.isArray(data) ? data[0] : data;
            if (typeof path === "string" && path) await openPath(path);
        });
        return () => {
            off();
        };
    }, [openPath]);

    return (
        <ConfigProvider
            locale={antdLocale}
            theme={{
                algorithm: isDarkMode ? theme.darkAlgorithm : theme.defaultAlgorithm,
                token: {colorPrimary: themeColor ?? DefaultThemeColor},
            }}
        >
            <AntdApp component={false}>
                <MessageBinder/>
                <Layout style={{height: "100vh"}}>
                    <NavBar onSelectFile={selectAndOpen}/>
                    <Content style={{padding: 16, overflow: "hidden", display: "flex", minHeight: 0}}>
                        <Routes>
                            <Route path="/" element={<HomePage/>}/>
                            <Route path="/container" element={<ContainerPage/>}/>
                            <Route path="/ct" element={<CtPage/>}/>
                            <Route path="/unpacked" element={<UnpackedPage/>}/>
                            <Route path="/pack" element={<PackPage/>}/>
                            <Route path="/settings" element={<SettingsPage/>}/>
                        </Routes>
                    </Content>
                </Layout>
            </AntdApp>
        </ConfigProvider>
    );
};

export default App;
