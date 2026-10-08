"use client";
import { useParams } from "next/navigation";
import { ServerDetail } from "../../../components/server-detail";
export default function Page() {
  const { id } = useParams();
  return <ServerDetail id={id} />;
}
