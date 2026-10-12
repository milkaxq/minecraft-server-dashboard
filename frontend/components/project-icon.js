"use client";
import Image from "next/image";
import { useState } from "react";
import { Icon } from "./ui";

export function ProjectIcon({ project }) {
  const [failedURL, setFailedURL] = useState(null);
  let safe = false;
  try {
    const url = new URL(project.icon_url);
    safe =
      url.protocol === "https:" &&
      url.host === "cdn.modrinth.com" &&
      !url.username &&
      !url.password &&
      !url.hash &&
      /\.(png|jpe?g|webp|gif)$/i.test(url.pathname);
  } catch {
    safe = false;
  }
  return (
    <span className="project-icon" aria-hidden="true">
      {safe && failedURL !== project.icon_url ? (
        <Image
          src={project.icon_url}
          alt=""
          width={56}
          height={56}
          unoptimized
          referrerPolicy="no-referrer"
          onError={() => setFailedURL(project.icon_url)}
        />
      ) : (
        <Icon name="server" size={24} />
      )}
    </span>
  );
}
