<template>
    <q-layout class="workspace">
        <q-page-container>
            <q-page class="flex flex-center">
                <section class="not-found page-heading">
                    <q-icon name="wrong_location" color="primary" size="42px" />
                    <h1>找不到这个页面</h1>
                    <p>请检查访问地址，或返回域名工作台。</p>
                    <q-btn unelevated no-caps color="primary" icon="arrow_back" label="返回首页" to="/" />
                </section>
            </q-page>
        </q-page-container>
    </q-layout>
</template>
<style scoped>
.not-found { max-width: 34rem; padding: 28px; text-align: center; }
.not-found h1 { margin-top: 12px; }
.not-found .q-btn { margin-top: 20px; }
</style>
