import { AddonSettings } from "../../../../../components/addon-settings";
export default async function Page({ params }) {
  const { id } = await params;
  return <AddonSettings id={id} />;
}
