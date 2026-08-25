import type { CSSProperties } from "react";
import type { Point, RealmStage } from "../realmguard/types";
import { defenseMapLabel } from "./maps";

const MAP_WIDTH = 1280;
const MAP_HEIGHT = 720;
const MARKER_MARGIN = 24;

interface MapPalette {
  background: string;
  field: string;
  path: string;
  pathEdge: string;
  accent: string;
  spot: string;
}

const MAP_PALETTES: Record<RealmStage["theme"], MapPalette> = {
  verdant: {
    background: "#0b2526",
    field: "#17443d",
    path: "#887c62",
    pathEdge: "#07151c",
    accent: "#72e0a6",
    spot: "#b8f5dc",
  },
  ember: {
    background: "#261a22",
    field: "#4a2d2c",
    path: "#866557",
    pathEdge: "#170f18",
    accent: "#ff9b6b",
    spot: "#ffd2b8",
  },
  frost: {
    background: "#102433",
    field: "#244859",
    path: "#718990",
    pathEdge: "#071723",
    accent: "#65d6ff",
    spot: "#c8f3ff",
  },
  void: {
    background: "#17162b",
    field: "#302b4b",
    path: "#6d6880",
    pathEdge: "#0c0c1b",
    accent: "#b694ff",
    spot: "#e5d8ff",
  },
};

export interface StageMapPreviewProps {
  stage: RealmStage;
  label?: string;
  className?: string;
  style?: CSSProperties;
}

function visiblePaths(stage: RealmStage): Point[][] {
  const paths = stage.paths?.filter((path) => path.length >= 2) ?? [];
  if (paths.length) return paths;
  return stage.path.length >= 2 ? [stage.path] : [];
}

function svgPoints(path: Point[]): string {
  return path.map((point) => `${point.x},${point.y}`).join(" ");
}

function visibleMarker(point: Point): Point {
  return {
    x: Math.min(MAP_WIDTH - MARKER_MARGIN, Math.max(MARKER_MARGIN, point.x)),
    y: Math.min(MAP_HEIGHT - MARKER_MARGIN, Math.max(MARKER_MARGIN, point.y)),
  };
}

function uniqueMarkers(points: Point[]): Point[] {
  const markers = new Map<string, Point>();
  for (const point of points) {
    const visible = visibleMarker(point);
    markers.set(`${visible.x}:${visible.y}`, visible);
  }
  return [...markers.values()];
}

/** A lightweight, asset-free overview of a playable Defense stage. */
export function StageMapPreview({
  stage,
  label,
  className,
  style,
}: StageMapPreviewProps) {
  const paths = visiblePaths(stage);
  const palette = MAP_PALETTES[stage.theme];
  const mapLabel =
    label ?? (stage.mapStyle ? defenseMapLabel(stage.mapStyle) : stage.name);
  const starts = uniqueMarkers(
    paths.flatMap((path) => (path[0] ? [path[0]] : [])),
  );
  const gates = uniqueMarkers(
    paths.flatMap((path) => (path.at(-1) ? [path.at(-1)!] : [])),
  );

  return (
    <svg
      className={className}
      data-map-style={stage.mapStyle ?? "default"}
      data-testid={`stage-map-preview-${stage.id}`}
      viewBox={`0 0 ${MAP_WIDTH} ${MAP_HEIGHT}`}
      preserveAspectRatio="xMidYMid meet"
      role="img"
      aria-label={`${stage.name} · ${mapLabel} 지도 미리보기`}
      style={{
        display: "block",
        width: "100%",
        height: "auto",
        overflow: "hidden",
        borderRadius: 12,
        ...style,
      }}
    >
      <title>{`${stage.name} · ${mapLabel}`}</title>
      <rect width={MAP_WIDTH} height={MAP_HEIGHT} fill={palette.background} />
      <rect
        x={14}
        y={14}
        width={MAP_WIDTH - 28}
        height={MAP_HEIGHT - 28}
        rx={28}
        fill={palette.field}
        stroke={palette.accent}
        strokeOpacity={0.2}
        strokeWidth={4}
      />

      <g aria-hidden="true" opacity={0.12} stroke={palette.accent} strokeWidth={2}>
        {[240, 480, 720, 960].map((x) => (
          <line key={`vertical-${x}`} x1={x} y1={20} x2={x} y2={700} />
        ))}
        {[180, 360, 540].map((y) => (
          <line key={`horizontal-${y}`} x1={20} y1={y} x2={1260} y2={y} />
        ))}
      </g>

      <g data-testid="stage-map-paths">
        {paths.map((path, index) => (
          <g key={`path-${index}`} data-lane={index + 1}>
            <polyline
              points={svgPoints(path)}
              fill="none"
              stroke={palette.pathEdge}
              strokeOpacity={0.65}
              strokeWidth={76}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <polyline
              points={svgPoints(path)}
              fill="none"
              stroke={palette.path}
              strokeWidth={56}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <polyline
              points={svgPoints(path)}
              fill="none"
              stroke="#fff4d4"
              strokeOpacity={0.22}
              strokeWidth={4}
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </g>
        ))}
      </g>

      <g data-testid="stage-map-tower-spots">
        {stage.towerSpots.map((spot) => (
          <g key={spot.id} transform={`translate(${spot.x} ${spot.y})`}>
            <circle r={29} fill={palette.background} fillOpacity={0.9} />
            <circle
              r={25}
              fill="none"
              stroke={palette.spot}
              strokeOpacity={0.9}
              strokeWidth={5}
            />
            <path
              d="M -10 0 H 10 M 0 -10 V 10"
              fill="none"
              stroke={palette.spot}
              strokeWidth={5}
              strokeLinecap="round"
            />
          </g>
        ))}
      </g>

      <g data-testid="stage-map-starts">
        {starts.map((point) => (
          <g key={`${point.x}:${point.y}`} transform={`translate(${point.x} ${point.y})`}>
            <circle
              r={24}
              fill={palette.background}
              stroke={palette.accent}
              strokeWidth={6}
            />
            <path d="M -7 -11 L 12 0 L -7 11 Z" fill={palette.accent} />
          </g>
        ))}
      </g>

      <g data-testid="stage-map-gates">
        {gates.map((point) => (
          <g key={`${point.x}:${point.y}`} transform={`translate(${point.x} ${point.y})`}>
            <rect
              x={-23}
              y={-23}
              width={46}
              height={46}
              rx={9}
              fill={palette.background}
              stroke={palette.accent}
              strokeWidth={6}
            />
            <path
              d="M -10 12 V -5 Q 0 -18 10 -5 V 12"
              fill="none"
              stroke={palette.accent}
              strokeWidth={6}
              strokeLinecap="round"
            />
          </g>
        ))}
      </g>

      <g aria-hidden="true">
        <rect
          x={34}
          y={32}
          width={440}
          height={88}
          rx={18}
          fill={palette.background}
          fillOpacity={0.9}
          stroke={palette.accent}
          strokeOpacity={0.35}
          strokeWidth={3}
        />
        <text
          x={58}
          y={70}
          fill="#ffffff"
          fontFamily="system-ui, sans-serif"
          fontSize={25}
          fontWeight={800}
        >
          {mapLabel}
        </text>
        <text
          x={58}
          y={101}
          fill={palette.spot}
          fontFamily="system-ui, sans-serif"
          fontSize={18}
          fontWeight={700}
        >
          {`${paths.length} LANE · ${stage.towerSpots.length} SPOTS`}
        </text>
      </g>
    </svg>
  );
}

export default StageMapPreview;
