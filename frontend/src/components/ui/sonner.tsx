"use client"

import { useSyncExternalStore } from "react"
import { Toaster as Sonner, type ToasterProps } from "sonner"
import { CircleCheckIcon, InfoIcon, TriangleAlertIcon, OctagonXIcon, Loader2Icon } from "lucide-react"

const MOBILE_QUERY = "(max-width: 639px)"

// sonner 的 position 不支持响应式：窄屏顶部居中，桌面右下角，避免挡住顶部导航与右上角主操作。
function subscribeViewportChange(onChange: () => void) {
  // jsdom 等环境没有 matchMedia：按桌面端处理
  if (typeof window.matchMedia !== "function") return () => {}
  const query = window.matchMedia(MOBILE_QUERY)
  query.addEventListener("change", onChange)
  return () => query.removeEventListener("change", onChange)
}

function getIsMobileViewport() {
  return typeof window.matchMedia === "function" && window.matchMedia(MOBILE_QUERY).matches
}

const Toaster = ({ ...props }: ToasterProps) => {
  const isMobileViewport = useSyncExternalStore(subscribeViewportChange, getIsMobileViewport)

  return (
    <Sonner
      theme="light"
      closeButton
      position={isMobileViewport ? "top-center" : "bottom-right"}
      offset={16}
      mobileOffset={16}
      richColors
      className="toaster group"
      icons={{
        success: (
          <CircleCheckIcon className="size-4" />
        ),
        info: (
          <InfoIcon className="size-4" />
        ),
        warning: (
          <TriangleAlertIcon className="size-4" />
        ),
        error: (
          <OctagonXIcon className="size-4" />
        ),
        loading: (
          <Loader2Icon className="size-4 animate-spin" />
        ),
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "calc(var(--radius) * 0.8)",
        } as React.CSSProperties
      }
      toastOptions={{
        classNames: {
          toast:
            "rounded-2xl border border-border bg-popover text-popover-foreground shadow-[var(--shadow-soft)]",
          title: "font-medium tracking-[-0.02em]",
          description: "text-sm text-muted-foreground",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
